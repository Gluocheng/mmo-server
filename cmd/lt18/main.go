package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	pomeloClient "github.com/cherry-game/cherry/net/parser/pomelo/client"
	pomeloMessage "github.com/cherry-game/cherry/net/parser/pomelo/message"
	"github.com/example/mmo-server/internal/protocol"
)

// LT-18 装备穿脱：50 人穿上、换下、脱下，属性变化在下一拍。
// 道具：2001 新手木剑(部位1 武器 attack+5 crit_rate+10)、2002 布衣(部位4 衣服 hp+20 defense+3 damage_reduction+5)。
// 基础属性 max_hp=100 attack=10 defense=0。

const loadN = 50

var addr string

type slot struct {
	c        *pomeloClient.Client
	uid      int64
	playerID int64
	line     int32

	mu         sync.Mutex
	equips     int // 收到 onEquip 次数
	bags       int // 收到 onBagChange 次数
	statHits   int // 收到含自己 uid 的 CombatStat 次数
	lastStat   *protocol.CombatStat
	equipItems map[int32]int32 // slotID -> itemID（最近一次 onEquip）
	equipAttrs map[int32]int   // slotID -> 属性条数
	bagSlots   map[int32]int32 // 装备背包 slot -> itemID（最近一次 onBagChange）
}

func main() {
	flag.StringVar(&addr, "addr", "gate-entry:10100", "gate entry host:port")
	flag.Parse()
	if err := waitReady(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if !fullChain() || !load() {
		os.Exit(1)
	}
}

func fullChain() bool {
	fmt.Println("== 全链路 2 人穿脱 ==")
	slots := make([]*slot, 2)
	if !enterArena(slots, "fc") {
		closeAll(slots)
		return false
	}
	defer closeAll(slots)
	ok := true
	a := slots[0]
	// 发两件装备到装备背包(bag_type=4)，各占一格：slot0=2001 slot1=2002
	ok = ok && addItems(a, 2001, 1) && addItems(a, 2002, 1)
	time.Sleep(300 * time.Millisecond)

	code, lat := wear(a, 4, 0, 0)
	st := a.lastEquip()
	fmt.Printf("穿木剑 code=%d 耗时=%s 部位1道具=%d attack属性条=%d\n", code, lat.Round(time.Millisecond), st.slotItem(1), st.attrCount(1))
	ok = ok && code == 0 && st.slotItem(1) == 2001

	// 再发一件木剑，穿到同一部位应换下旧的（旧的回背包格）
	ok = ok && addItems(a, 2001, 1)
	time.Sleep(200 * time.Millisecond)
	slotOf := a.bagSlotOfItem(2001)
	code, lat = wear(a, 4, slotOf, 0)
	fmt.Printf("换下木剑 code=%d 耗时=%s 部位1道具=%d\n", code, lat.Round(time.Millisecond), a.lastEquip().slotItem(1))
	ok = ok && code == 0 && a.lastEquip().slotItem(1) == 2001

	// 穿上布衣到部位 4
	ok = ok && addItems(a, 2002, 1)
	time.Sleep(200 * time.Millisecond)
	code, _ = wear(a, 4, a.bagSlotOfItem(2002), 0)
	fmt.Printf("穿布衣 code=%d 部位4道具=%d\n", code, a.lastEquip().slotItem(4))
	ok = ok && code == 0 && a.lastEquip().slotItem(4) == 2002

	// 下一拍属性变化：attack 10+5=15，max_hp 100+20=120，defense 0+3=3
	time.Sleep(500 * time.Millisecond)
	s := a.stat()
	fmt.Printf("属性帧 attack=%d max_hp=%d defense=%d crit_rate=%d damage_reduction=%d\n", s.attack, s.maxHP, s.defense, s.critRate, s.damageReduction)
	ok = ok && s.attack == 15 && s.maxHP == 120 && s.defense == 3 && s.critRate == 10 && s.damageReduction == 5

	// 脱下武器：attack 回到 10
	code, _ = unwear(a, 1)
	time.Sleep(500 * time.Millisecond)
	s = a.stat()
	fmt.Printf("脱下武器 code=%d 后 attack=%d 部位1道具=%d\n", code, s.attack, a.lastEquip().slotItem(1))
	ok = ok && code == 0 && s.attack == 10 && a.lastEquip().slotItem(1) == 0

	// 未穿部位脱下返回 40067
	code, _ = unwear(a, 8)
	fmt.Printf("脱下未穿部位 code=%d\n", code)
	ok = ok && code == 40067

	// 对方全程没穿装备，属性不应变化
	b := slots[1]
	bs := b.stat()
	fmt.Printf("对照乙 attack=%d max_hp=%d（应为 10/100，收到属性帧=%d）\n", bs.attack, bs.maxHP, b.statCount())
	ok = ok && b.statCount() == 0

	if ok {
		fmt.Println("全链路通过")
	}
	return ok
}

func load() bool {
	fmt.Printf("== 压测 %d 人穿脱 ==\n", loadN)
	slots := make([]*slot, loadN)
	if !enterArena(slots, "ld") {
		closeAll(slots)
		return false
	}
	defer closeAll(slots)
	ok := true

	// 逐人发装备（避免并发 add 的旧死锁问题；背包已内存化，仍按 lt09 惯例逐个发）
	start := time.Now()
	added := 0
	for _, s := range slots {
		if addItems(s, 2001, 1) {
			added++
		}
	}
	fmt.Printf("发木剑 %d/%d 墙钟=%s\n", added, loadN, time.Since(start).Round(time.Millisecond))
	ok = ok && added == loadN
	time.Sleep(300 * time.Millisecond)

	// 同时穿上
	for _, s := range slots {
		s.reset()
	}
	codes := make([]int, loadN)
	lats := make([]time.Duration, loadN)
	start = time.Now()
	parallel(loadN, func(i int) {
		codes[i], lats[i] = wear(slots[i], 4, 0, 0)
	})
	printStats("wear", time.Since(start), codes, lats)
	ok = ok && count(codes, 0) == loadN

	// 下一拍属性：attack=15 crit_rate=10
	time.Sleep(600 * time.Millisecond)
	attrOK := 0
	pushOK := 0
	for _, s := range slots {
		st := s.stat()
		if st.attack == 15 && st.critRate == 10 {
			attrOK++
		}
		if s.equipPushes() >= 1 {
			pushOK++
		}
	}
	fmt.Printf("穿上后属性对=%d onEquip推送=%d\n", attrOK, pushOK)
	ok = ok && attrOK == loadN && pushOK == loadN

	// 同时脱下
	for _, s := range slots {
		s.reset()
	}
	codes = make([]int, loadN)
	lats = make([]time.Duration, loadN)
	start = time.Now()
	parallel(loadN, func(i int) {
		codes[i], lats[i] = unwear(slots[i], 1)
	})
	printStats("unwear", time.Since(start), codes, lats)
	ok = ok && count(codes, 0) == loadN

	time.Sleep(600 * time.Millisecond)
	attrOK = 0
	for _, s := range slots {
		if st := s.stat(); st.attack == 10 && st.critRate == 0 {
			attrOK++
		}
	}
	fmt.Printf("脱下后属性回基础=%d\n", attrOK)
	ok = ok && attrOK == loadN

	if ok {
		fmt.Println("压测通过")
	}
	return ok
}

// enterArena 进场主城并切演武场（战斗场景才有属性帧）。
func enterArena(slots []*slot, tag string) bool {
	n := len(slots)
	if !enterMain(slots, tag) {
		return false
	}
	codes := make([]int, n)
	lats := make([]time.Duration, n)
	start := time.Now()
	parallel(n, func(i int) {
		var rsp protocol.SceneSwitchResponse
		t := time.Now()
		code, err := call(slots[i].c, "game.player.switchScene", &protocol.SceneSwitchRequest{SceneId: 2}, &rsp)
		lats[i] = time.Since(t)
		if err != nil || rsp.SceneId != 2 {
			if code == 0 {
				code = -5
			}
			codes[i] = code
			return
		}
		slots[i].line = rsp.Line
	})
	printStats("switch-arena", time.Since(start), codes, lats)
	return count(codes, 0) == n
}

func enterMain(slots []*slot, tag string) bool {
	n := len(slots)
	codes := make([]int, n)
	stamp := time.Now().UnixNano()
	start := time.Now()
	parallel(n, func(i int) {
		s := &slot{}
		slots[i] = s
		c := pomeloClient.New(pomeloClient.WithRequestTimeout(20 * time.Second))
		c.On("onEquip", s.onEquip)
		c.On("onBagChange", s.onBag)
		c.On("onCombatFrame", s.onFrame)
		if err := c.ConnectToWS(addr, ""); err != nil {
			codes[i] = -1
			return
		}
		s.c = c
		var issued protocol.IssueTokenResponse
		code, err := call(c, "gate.user.issueToken", &protocol.IssueTokenRequest{
			Nickname: fmt.Sprintf("%s%d-%d", tag, stamp, i),
			Password: "123456",
			DeviceId: "lt18",
		}, &issued)
		if err != nil || issued.AccessToken == "" {
			if code == 0 {
				code = -3
			}
			codes[i] = code
			return
		}
		var login protocol.TokenLoginResponse
		code, err = call(c, "gate.user.login", &protocol.TokenLoginRequest{
			AccessToken: issued.AccessToken, ServerId: 10001, DeviceId: "lt18",
		}, &login)
		if err != nil || login.Uid < 1 {
			if code == 0 {
				code = -3
			}
			codes[i] = code
			return
		}
		s.uid = login.Uid
		var created protocol.PlayerCreateResponse
		_, _ = call(c, "game.player.select", &protocol.None{}, &protocol.PlayerSelectResponse{})
		code, err = call(c, "game.player.create", &protocol.PlayerCreateRequest{
			Name: fmt.Sprintf("%s%d-%d", tag, stamp%100000, i),
		}, &created)
		if err != nil || created.Player == nil {
			if code == 0 {
				code = -3
			}
			codes[i] = code
			return
		}
		s.playerID = created.Player.PlayerId
		var entered protocol.EnterGameResponse
		code, err = call(c, "game.player.enter", &protocol.EnterGameRequest{
			PlayerId: s.playerID, SceneId: 1,
		}, &entered)
		if err != nil || entered.SceneId != 1 {
			if code == 0 {
				code = -5
			}
			codes[i] = code
		}
	})
	printStats("enter", time.Since(start), codes, nil)
	return count(codes, 0) == n
}

func addItems(s *slot, itemID, n int32) bool {
	code, err := call(s.c, "game.bag.add", &protocol.BagAddRequest{ItemId: itemID, Count: n}, &protocol.BagListResponse{})
	return err == nil && code == 0
}

func wear(s *slot, bagType, bagSlot, targetSlot int32) (int, time.Duration) {
	start := time.Now()
	code, err := call(s.c, "game.equip.wear", &protocol.EquipWearRequest{
		BagType: bagType, Slot: bagSlot, TargetSlot: targetSlot,
	}, &protocol.EquipState{})
	if err != nil && code == 0 {
		code = -2
	}
	return code, time.Since(start)
}

func unwear(s *slot, slotID int32) (int, time.Duration) {
	start := time.Now()
	code, err := call(s.c, "game.equip.unwear", &protocol.EquipUnwearRequest{SlotId: slotID}, &protocol.EquipState{})
	if err != nil && code == 0 {
		code = -2
	}
	return code, time.Since(start)
}

type equipView struct {
	items map[int32]int32 // slotID -> itemID
	attrs map[int32]int   // slotID -> 属性条数
}

func (v equipView) slotItem(slotID int32) int32 { return v.items[slotID] }
func (v equipView) attrCount(slotID int32) int  { return v.attrs[slotID] }

type statView struct {
	attack, maxHP, defense                  int32
	critRate, critDamage, penetrate         int32
	damageBonus, damageReduction, lifesteal int32
}

func (s *slot) onEquip(msg *pomeloMessage.Message) {
	if len(msg.Data) == 0 {
		return
	}
	var st protocol.EquipState
	if s.c.Serializer().Unmarshal(msg.Data, &st) != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.equips++
	s.applyEquip(&st)
}

func (s *slot) applyEquip(st *protocol.EquipState) {
	s.equipItems = map[int32]int32{}
	s.equipAttrs = map[int32]int{}
	for _, sl := range st.Slots {
		if sl == nil {
			continue
		}
		s.equipItems[sl.SlotId] = sl.ItemId
		s.equipAttrs[sl.SlotId] = len(sl.Attrs)
	}
}

func (s *slot) onBag(msg *pomeloMessage.Message) {
	if len(msg.Data) == 0 {
		return
	}
	var bag protocol.BagListResponse
	if s.c.Serializer().Unmarshal(msg.Data, &bag) != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bags++
	s.bagSlots = map[int32]int32{} // slot -> itemID（装备背包）
	for _, it := range bag.Items {
		if it != nil && it.BagType == 4 {
			s.bagSlots[it.Slot] = it.ItemId
		}
	}
}

func (s *slot) onFrame(msg *pomeloMessage.Message) {
	if len(msg.Data) == 0 {
		return
	}
	var f protocol.CombatFrame
	if s.c.Serializer().Unmarshal(msg.Data, &f) != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, st := range f.Stats {
		if st != nil && st.Uid == s.uid {
			s.statHits++
			s.lastStat = st
		}
	}
}

// 需要加锁访问的字段在 struct 上直接放，简化：equipItems/equipAttrs/bagSlots
func (s *slot) lastEquip() equipView {
	s.mu.Lock()
	defer s.mu.Unlock()
	return equipView{items: copyI32(s.equipItems), attrs: copyI32i(s.equipAttrs)}
}

func (s *slot) bagSlotOfItem(itemID int32) int32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	keys := make([]int32, 0, len(s.bagSlots))
	for k := range s.bagSlots {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	for _, k := range keys {
		if s.bagSlots[k] == itemID {
			return k
		}
	}
	return 0
}

func (s *slot) stat() statView {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastStat == nil {
		return statView{}
	}
	return statView{
		attack: s.lastStat.Attack, maxHP: s.lastStat.MaxHp, defense: s.lastStat.Defense,
		critRate: s.lastStat.CritRate, critDamage: s.lastStat.CritDamage, penetrate: s.lastStat.Penetrate,
		damageBonus: s.lastStat.DamageBonus, damageReduction: s.lastStat.DamageReduction, lifesteal: s.lastStat.Lifesteal,
	}
}

func (s *slot) statCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statHits
}

func (s *slot) equipPushes() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.equips
}

func (s *slot) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.equips = 0
	s.bags = 0
	s.statHits = 0
	s.lastStat = nil
}

func copyI32(m map[int32]int32) map[int32]int32 {
	out := make(map[int32]int32, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func copyI32i(m map[int32]int) map[int32]int {
	out := make(map[int32]int, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func parallel(n int, fn func(i int)) {
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			fn(i)
		}(i)
	}
	wg.Wait()
}

func waitReady() error {
	deadline := time.Now().Add(60 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		c := pomeloClient.New(pomeloClient.WithRequestTimeout(3 * time.Second))
		err := c.ConnectToWS(addr, "")
		if err == nil {
			c.Disconnect()
			fmt.Printf("入口已就绪 %s\n", addr)
			return nil
		}
		last = err
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("入口未就绪: %v", last)
}

func call(c *pomeloClient.Client, route string, req, rsp interface{}) (int, error) {
	msg, err := c.Request(route, req)
	if err != nil {
		return statusCode(err), err
	}
	if rsp != nil && len(msg.Data) > 0 {
		if err := c.Serializer().Unmarshal(msg.Data, rsp); err != nil {
			return -2, err
		}
	}
	return 0, nil
}

func closeAll(slots []*slot) {
	for _, s := range slots {
		if s != nil && s.c != nil {
			s.c.Disconnect()
		}
	}
}

func printStats(name string, wall time.Duration, codes []int, lats []time.Duration) {
	okN := count(codes, 0)
	var okLats []time.Duration
	if lats != nil {
		for i, c := range codes {
			if c == 0 {
				okLats = append(okLats, lats[i])
			}
		}
		sort.Slice(okLats, func(i, j int) bool { return okLats[i] < okLats[j] })
	}
	fmt.Printf("%s 成功=%d 失败=%d 墙钟=%s 返回码 %s\n", name, okN, len(codes)-okN, wall.Round(time.Millisecond), formatCounts(codes))
	if len(okLats) > 0 {
		fmt.Printf("成功耗时 p50=%s p95=%s max=%s\n", pct(okLats, 50), pct(okLats, 95), okLats[len(okLats)-1].Round(time.Millisecond))
	}
}

func count(codes []int, want int) int {
	n := 0
	for _, c := range codes {
		if c == want {
			n++
		}
	}
	return n
}

func statusCode(err error) int {
	const key = "statusCode = "
	text := err.Error()
	i := strings.Index(text, key)
	if i < 0 {
		return -1
	}
	rest := text[i+len(key):]
	end := strings.IndexByte(rest, ',')
	if end < 0 {
		end = len(rest)
	}
	n, conv := strconv.Atoi(strings.TrimSpace(rest[:end]))
	if conv != nil {
		return -1
	}
	return n
}

func pct(sorted []time.Duration, p int) time.Duration {
	idx := (len(sorted)*p + 99) / 100
	if idx < 1 {
		idx = 1
	}
	if idx > len(sorted) {
		idx = len(sorted)
	}
	return sorted[idx-1].Round(time.Millisecond)
}

func formatCounts(codes []int) string {
	counts := map[int]int{}
	for _, c := range codes {
		counts[c]++
	}
	keys := make([]int, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%d×%d", k, counts[k]))
	}
	return strings.Join(parts, " ")
}
