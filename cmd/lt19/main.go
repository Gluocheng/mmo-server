// LT-19 装备强化压测：50 人强化身上的装备，覆盖成功、失败掉级和保护符。
// 强化 1-7 级必成，8 级起 70/50/30 成功率，失败按 50/70/100 掉级。
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

const (
	loadN      = 50
	swordItem  = int32(2001)
	equipBag   = int32(4)
	matBag     = int32(3)
	stoneItem  = int32(1003)
	slotWeapon = int32(1)
)

var addr string

type slot struct {
	c        *pomeloClient.Client
	uid      int64
	playerID int64
	line     int32
	mu       sync.Mutex
	pushes   int
	lastBag  map[int32]int32 // itemId -> count
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
	fmt.Println("== 全链路 2 人强化 ==")
	slots := make([]*slot, 2)
	if !enterMain(slots, "fc") {
		closeAll(slots)
		return false
	}
	defer closeAll(slots)
	ok := true
	for i, s := range slots {
		if i == 0 {
			// 甲发足量石头；乙只发 1 颗，用来验证石头不足
			if !give(s, stoneItem, matBag, 100) {
				ok = false
			}
		} else {
			if !give(s, stoneItem, matBag, 1) {
				ok = false
			}
		}
		if !give(s, swordItem, equipBag, 1) {
			ok = false
		}
		state, c := wear(s, 0, 0)
		fmt.Printf("穿剑 %d code=%d 部位1道具=%d\n", i, c, slotItem(state, slotWeapon))
		ok = ok && c == 0 && slotItem(state, slotWeapon) == swordItem
	}
	a := slots[0]
	// 1 级必成
	rsp, c, lat := enhance(a, slotWeapon, false)
	fmt.Printf("甲强化1 code=%d success=%v level=%d 耗时=%s\n", c, rsp.Success, rsp.Level, lat.Round(time.Millisecond))
	ok = ok && c == 0 && rsp.Success && rsp.Level == 1
	// 没穿的部位强化 40069
	_, c, _ = enhance(a, 5, false)
	fmt.Printf("未穿部位 code=%d\n", c)
	ok = ok && c == 40069
	// 非法部位 40067
	_, c, _ = enhance(a, 99, false)
	fmt.Printf("非法部位 code=%d\n", c)
	ok = ok && c == 40067
	// 保护符不足 40019
	_, c, _ = enhance(a, slotWeapon, true)
	fmt.Printf("无保护符用保护 code=%d\n", c)
	ok = ok && c == 40019
	// 石头不足 40019：乙只有 1 颗石头，从 1 级强化到 2 需要 2 颗
	b := slots[1]
	_, c, _ = enhance(b, slotWeapon, false) // 0->1 用 1 颗，成功
	fmt.Printf("乙强化1 code=%d\n", c)
	ok = ok && c == 0
	_, c, _ = enhance(b, slotWeapon, false) // 1->2 需 2 颗，石头已用完
	fmt.Printf("乙石头不足 code=%d\n", c)
	ok = ok && c == 40019
	// 强化后 equips 查询带等级
	state, c := equips(a)
	ok = ok && c == 0 && slotLevel(state, slotWeapon) == 1
	fmt.Printf("查询等级=%d\n", slotLevel(state, slotWeapon))
	// 属性加成按等级抬升：attack flat 5，1 级 boosted = 5+max(1,5*1/10)=6
	for _, sl := range state.GetSlots() {
		if sl.SlotId == slotWeapon {
			for _, at := range sl.Attrs {
				if at.Stat == "attack" {
					fmt.Printf("1级攻击加成=%d\n", at.Value)
					ok = ok && at.Value == 6
				}
			}
		}
	}
	if ok {
		fmt.Println("全链路通过")
	}
	return ok
}

func load() bool {
	fmt.Printf("== 压测 %d 人同时强化 ==\n", loadN)
	slots := make([]*slot, loadN)
	if !enterMain(slots, "ld") {
		closeAll(slots)
		return false
	}
	defer closeAll(slots)
	ok := true
	// 每人发 200 颗石头 + 1 件剑并穿上（发道具逐个来，避免背包写放大）
	for _, s := range slots {
		if !give(s, stoneItem, matBag, 200) || !give(s, swordItem, equipBag, 1) {
			ok = false
		}
	}
	codes := make([]int, loadN)
	var wg sync.WaitGroup
	start := time.Now()
	for i := range slots {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, c := wear(slots[i], 0, 0)
			codes[i] = c
		}(i)
	}
	wg.Wait()
	printStats("wear", time.Since(start), codes, nil)
	ok = ok && count(codes, 0) == loadN

	// 第 1 轮：50 人同时强化到 1 级（必成）
	levels, lats, codes := enhanceAll(slots, false)
	ok = ok && count(codes, 0) == loadN && countN(levels, 1) == loadN
	fmt.Printf("第1轮 level=1 人数=%d\n", countN(levels, 1))

	// 第 2-7 轮：继续同时强化，1-7 级必成
	for round := 2; round <= 7; round++ {
		levels, _, codes = enhanceAll(slots, false)
		if count(codes, 0) != loadN || countN(levels, int32(round)) != loadN {
			fmt.Printf("第%d轮异常 返回码 %s level=%s\n", round, formatCounts(codes), formatN(levels))
			ok = false
			break
		}
	}
	fmt.Printf("第7轮后 level=7 人数=%d\n", countN(levels, 7))
	_ = lats

	// 8 级起有失败和掉级：再打 12 轮，统计成功/失败掉级/满级
	var succ, fail, drop, maxed, bad int
	transBad := 0
	for round := 0; round < 12; round++ {
		before := append([]int32(nil), levels...)
		levels, _, codes = enhanceAll(slots, false)
		for i := range slots {
			switch codes[i] {
			case 0:
			case 40068:
				maxed++
				continue
			default:
				bad++
				continue
			}
			b, a := before[i], levels[i]
			if a == b+1 {
				succ++
			} else if a == b {
				fail++
			} else if b > 0 && a == b-1 {
				drop++
			} else if b == 0 && a == 0 {
				fail++
			} else {
				transBad++
			}
		}
	}
	fmt.Printf("8级以上 成功=%d 失败不掉=%d 掉级=%d 满级40068=%d 非法等级迁移=%d 其他错误=%d\n",
		succ, fail, drop, maxed, transBad, bad)
	ok = ok && bad == 0 && transBad == 0 && succ+fail+drop+maxed > 0

	// 保护符：发保护符后同时带保护强化一轮，保护触发不掉级
	for _, s := range slots {
		if !give(s, protectItem(), matBag, 5) {
			ok = false
		}
	}
	before := append([]int32(nil), levels...)
	rspAll, codes := enhanceAllRsp(slots, true)
	protectBad := 0
	for i := range slots {
		if codes[i] == 40068 {
			continue
		}
		if codes[i] != 0 {
			protectBad++
			continue
		}
		r := rspAll[i]
		if r.Dropped {
			protectBad++ // 带保护不应掉级
		}
		if !r.Success && r.Level != before[i] && !r.ProtectUsed {
			protectBad++
		}
		if r.Level < before[i] {
			protectBad++
		}
	}
	fmt.Printf("带保护强化 异常=%d\n", protectBad)
	ok = ok && protectBad == 0

	// 满级路径：把一个人连打到 10 级再强化应 40068
	s0 := slots[0]
	reached := false
	for i := 0; i < 40; i++ {
		_, c, _ := enhance(s0, slotWeapon, false)
		if c == 40068 {
			reached = true
			break
		}
		if c == 40019 { // 石头不足补发
			give(s0, stoneItem, matBag, 200)
		}
	}
	// 满级时再查一次，顺手确认等级确实是 10
	state, sc := equips(s0)
	lv := slotLevel(state, slotWeapon)
	fmt.Printf("甲满级=%v 查询等级=%d 查询码=%d\n", reached, lv, sc)
	_, c, _ := enhance(s0, slotWeapon, false)
	ok = ok && reached && lv == 10 && c == 40068
	if ok {
		fmt.Println("压测通过")
	}
	return ok
}

func protectItem() int32 { return 1004 }

func enhanceAll(slots []*slot, useProtect bool) ([]int32, []time.Duration, []int) {
	rsp, codes := enhanceAllRsp(slots, useProtect)
	levels := make([]int32, len(slots))
	for i, r := range rsp {
		levels[i] = r.Level
	}
	return levels, nil, codes
}

func enhanceAllRsp(slots []*slot, useProtect bool) ([]*protocol.EquipEnhanceResponse, []int) {
	n := len(slots)
	rspAll := make([]*protocol.EquipEnhanceResponse, n)
	codes := make([]int, n)
	lats := make([]time.Duration, n)
	var wg sync.WaitGroup
	start := time.Now()
	for i := range slots {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rsp, c, lat := enhance(slots[i], slotWeapon, useProtect)
			rspAll[i], codes[i], lats[i] = rsp, c, lat
		}(i)
	}
	wg.Wait()
	printStats("enhance", time.Since(start), codes, lats)
	return rspAll, codes
}

func enhance(s *slot, slotID int32, useProtect bool) (*protocol.EquipEnhanceResponse, int, time.Duration) {
	start := time.Now()
	var rsp protocol.EquipEnhanceResponse
	code, err := call(s.c, "game.equip.enhance", &protocol.EquipEnhanceRequest{
		SlotId: slotID, UseProtect: useProtect,
	}, &rsp)
	if err != nil && code == 0 {
		code = -2
	}
	return &rsp, code, time.Since(start)
}

func wear(s *slot, bagSlot, target int32) (*protocol.EquipState, int) {
	var rsp protocol.EquipState
	code, err := call(s.c, "game.equip.wear", &protocol.EquipWearRequest{
		BagType: equipBag, Slot: bagSlot, TargetSlot: target,
	}, &rsp)
	if err != nil && code == 0 {
		code = -2
	}
	return &rsp, code
}

func equips(s *slot) (*protocol.EquipState, int) {
	var rsp protocol.EquipState
	code, err := call(s.c, "game.equip.equips", &protocol.None{}, &rsp)
	if err != nil && code == 0 {
		code = -2
	}
	return &rsp, code
}

func slotItem(state *protocol.EquipState, slotID int32) int32 {
	for _, sl := range state.GetSlots() {
		if sl.SlotId == slotID {
			return sl.ItemId
		}
	}
	return -1
}

func slotLevel(state *protocol.EquipState, slotID int32) int32 {
	for _, sl := range state.GetSlots() {
		if sl.SlotId == slotID {
			return sl.EnhanceLevel
		}
	}
	return -1
}

func give(s *slot, item int32, bag, n int32) bool {
	var rsp protocol.BagListResponse
	code, err := call(s.c, "game.bag.add", &protocol.BagAddRequest{
		ItemId: item, Count: n, BagType: bag,
	}, &rsp)
	if err != nil || code != 0 {
		fmt.Printf("发道具 %d code=%d err=%v\n", item, code, err)
		return false
	}
	return true
}

func enterMain(slots []*slot, tag string) bool {
	n := len(slots)
	codes := make([]int, n)
	stamp := time.Now().UnixNano()
	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s := &slot{lastBag: map[int32]int32{}}
			slots[i] = s
			c := pomeloClient.New(pomeloClient.WithRequestTimeout(20 * time.Second))
			c.On("onBagChange", s.onBag)
			if err := c.ConnectToWS(addr, ""); err != nil {
				codes[i] = -1
				return
			}
			s.c = c
			var issued protocol.IssueTokenResponse
			code, err := call(c, "gate.user.issueToken", &protocol.IssueTokenRequest{
				Nickname: fmt.Sprintf("%s%d-%d", tag, stamp, i),
				Password: "123456", DeviceId: "lt19",
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
				AccessToken: issued.AccessToken, ServerId: 10001, DeviceId: "lt19",
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
			if err != nil || entered.SceneId != 1 || entered.Line < 1 {
				if code == 0 {
					code = -5
				}
				codes[i] = code
				return
			}
			s.line = entered.Line
		}(i)
	}
	wg.Wait()
	printStats("enter", time.Since(start), codes, nil)
	if count(codes, 0) != n {
		return false
	}
	line := slots[0].line
	for _, s := range slots[1:] {
		if s.line != line {
			fmt.Println("不在同一条线")
			return false
		}
	}
	fmt.Printf("主城 %d 线 ×%d\n", line, n)
	return true
}

func (s *slot) onBag(msg *pomeloMessage.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pushes++
	if s.c == nil || len(msg.Data) == 0 {
		return
	}
	var bag protocol.BagListResponse
	if s.c.Serializer().Unmarshal(msg.Data, &bag) != nil {
		return
	}
	for _, it := range bag.Items {
		if it != nil {
			s.lastBag[it.ItemId] += it.Count
		}
	}
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

func countN(vals []int32, want int32) int {
	n := 0
	for _, v := range vals {
		if v == want {
			n++
		}
	}
	return n
}

func formatN(vals []int32) string {
	counts := map[int32]int{}
	for _, v := range vals {
		counts[v]++
	}
	keys := make([]int, 0, len(counts))
	for k := range counts {
		keys = append(keys, int(k))
	}
	sort.Ints(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%d×%d", k, counts[int32(k)]))
	}
	return strings.Join(parts, " ")
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
