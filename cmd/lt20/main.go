// LT-20 暴击与伤害修正：50 人穿上带暴击、暴伤、减伤、吸血的装备并互相点名。
// 配表推算：基础 attack=10 max_hp=100 defense=0。
// 穿 2001 木剑(attack+5, crit_rate+10)、2002 布衣(hp+20, defense+3, damage_reduction+5)、
// 2003 铜戒(attack+1, lifesteal+5)、2004 锋锐戒(crit_damage+20)：attack=16 maxHP=120 defense=3。
// 普攻(factor=100)：非暴击 (16-3)×95% = 12；暴击 (16×170/100-3)×95% = 22。
// 吸血 5%：非暴击 12×5%=0 无治疗帧；暴击 22×5%=1 有治疗帧。
// 配表没有穿透和伤害加成装备，这两项在本轮观察不到。
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

const loadN = 50

var addr string

var gear = []struct {
	item int32
	slot int32 // 部位
}{
	{2001, 1},  // 武器
	{2002, 4},  // 衣服
	{2003, 10}, // 左戒指
	{2004, 11}, // 右戒指（同为戒指组）
}

type slot struct {
	c    *pomeloClient.Client
	uid  int64
	line int32

	mu        sync.Mutex
	selfStat  *protocol.CombatStat
	hitsTaken []hitRec
	heals     []hitRec
}

type hitRec struct {
	amount int32
	crit   bool
	skill  int32
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
	fmt.Println("== 全链路 2 人穿装备点名 ==")
	slots := make([]*slot, 2)
	if !enterArena(slots, "fc") {
		closeAll(slots)
		return false
	}
	defer closeAll(slots)
	ok := true
	for i, s := range slots {
		if !dressUp(s) {
			fmt.Printf("穿装备 %d 失败\n", i)
			ok = false
		}
	}
	time.Sleep(400 * time.Millisecond)
	for i, s := range slots {
		st := s.self()
		fmt.Printf("甲%d 属性 attack=%d maxHP=%d defense=%d critRate=%d critDamage=%d reduction=%d lifesteal=%d\n",
			i, st.Attack, st.MaxHp, st.Defense, st.CritRate, st.CritDamage, st.DamageReduction, st.Lifesteal)
		ok = ok && st.Attack == 16 && st.MaxHp == 120 && st.Defense == 3 &&
			st.CritRate == 10 && st.CritDamage == 20 && st.DamageReduction == 5 && st.Lifesteal == 5
	}
	for _, s := range slots {
		s.resetHits()
	}
	c, lat := cast(slots[0], 1, slots[1].uid)
	fmt.Printf("点名 code=%d 耗时=%s\n", c, lat.Round(time.Millisecond))
	ok = ok && c == 0
	time.Sleep(500 * time.Millisecond)
	taken := slots[1].taken()
	fmt.Printf("目标受击 %+v\n", taken)
	ok = ok && len(taken) == 1 && checkAmount(taken[0])
	if len(taken) == 1 && taken[0].crit {
		heals := slots[0].healList()
		fmt.Printf("暴击吸血治疗帧 %+v\n", heals)
		ok = ok && len(heals) == 1 && heals[0].amount == 1
	}
	if ok {
		fmt.Println("全链路通过")
	}
	return ok
}

func load() bool {
	fmt.Printf("== 压测 %d 人穿装备互相点名 ==\n", loadN)
	slots := make([]*slot, loadN)
	if !enterArena(slots, "ld") {
		closeAll(slots)
		return false
	}
	defer closeAll(slots)
	ok := true
	for i, s := range slots {
		if !dressUp(s) {
			fmt.Printf("穿装备 %d 失败\n", i)
			ok = false
		}
	}
	// 等属性帧落地，校验 50 人属性一致
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		n := 0
		for _, s := range slots {
			st := s.self()
			if st.Attack == 16 && st.MaxHp == 120 {
				n++
			}
		}
		if n == loadN {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	attrOK, attrBad := 0, 0
	for _, s := range slots {
		st := s.self()
		if st.Attack == 16 && st.MaxHp == 120 && st.Defense == 3 &&
			st.CritRate == 10 && st.CritDamage == 20 && st.DamageReduction == 5 && st.Lifesteal == 5 {
			attrOK++
		} else {
			attrBad++
		}
	}
	fmt.Printf("属性帧到位=%d 异常=%d\n", attrOK, attrBad)
	ok = ok && attrOK == loadN

	// 两轮互相点名，间隔冷却
	totalHits, totalCrit, badAmount, healNonCrit, noHit := 0, 0, 0, 0, 0
	// 攻击方满血时 leech 不产生帧（missing=0），只有受过伤的人才期望回血
	healExpected, healGot := 0, 0
	for round := 0; round < 2; round++ {
		if round > 0 {
			time.Sleep(1200 * time.Millisecond)
		}
		for _, s := range slots {
			s.resetHits()
		}
		codes := make([]int, loadN)
		lats := make([]time.Duration, loadN)
		var wg sync.WaitGroup
		start := time.Now()
		for i := range slots {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				c, lat := cast(slots[i], 1, slots[(i+1)%loadN].uid)
				codes[i], lats[i] = c, lat
			}(i)
		}
		wg.Wait()
		printStats(fmt.Sprintf("cast%d", round+1), time.Since(start), codes, lats)
		ok = ok && count(codes, 0) == loadN
		time.Sleep(600 * time.Millisecond)
		for i, s := range slots {
			taken := s.taken()
			if len(taken) != 1 {
				noHit++
				continue
			}
			totalHits++
			h := taken[0]
			if !checkAmount(h) {
				badAmount++
			}
			attacker := slots[(i-1+loadN)%loadN]
			heals := attacker.healList()
			if h.crit {
				totalCrit++
				// 攻击方是否满血决定要不要回血：没满血才期望 1 点治疗
				if attacker.hp() < attacker.maxHP() {
					healExpected++
					if len(heals) == 1 && heals[0].amount == 1 {
						healGot++
					}
				}
			} else {
				// 非暴击 12×5%=0，无论是否满血都不该有治疗帧
				if len(heals) == 0 {
					healNonCrit++
				}
			}
		}
	}
	fmt.Printf("两轮合计 受击=%d 暴击=%d 非暴击=%d 伤害异常=%d 没收到受击=%d\n",
		totalHits, totalCrit, totalHits-totalCrit, badAmount, noHit)
	fmt.Printf("暴击且攻击方非满血=%d 其中收到吸血帧=%d 非暴击无吸血=%d\n",
		healExpected, healGot, healNonCrit)
	ok = ok && badAmount == 0 && noHit == 0 && healGot == healExpected && healNonCrit == totalHits-totalCrit
	if ok {
		fmt.Println("压测通过")
	}
	return ok
}

// checkAmount 非暴击 12、暴击 22，且 crit 标志与数值一致。
func checkAmount(h hitRec) bool {
	if h.skill != 1 {
		return false
	}
	if h.crit {
		return h.amount == 22
	}
	return h.amount == 12
}

// dressUp 发四件装备并按部位穿上。
func dressUp(s *slot) bool {
	for _, g := range gear {
		var rsp protocol.BagListResponse
		code, err := call(s.c, "game.bag.add", &protocol.BagAddRequest{ItemId: g.item, Count: 1}, &rsp)
		if err != nil || code != 0 {
			fmt.Printf("发 %d code=%d\n", g.item, code)
			return false
		}
		bagSlot := int32(-1)
		for _, it := range rsp.Items {
			if it != nil && it.ItemId == g.item {
				bagSlot = it.Slot
				break
			}
		}
		if bagSlot < 0 {
			return false
		}
		code, err = call(s.c, "game.equip.wear", &protocol.EquipWearRequest{
			BagType: 4, Slot: bagSlot, TargetSlot: g.slot,
		}, &protocol.EquipState{})
		if err != nil || code != 0 {
			fmt.Printf("穿 %d code=%d\n", g.item, code)
			return false
		}
	}
	return true
}

func cast(s *slot, skill int32, target int64) (int, time.Duration) {
	start := time.Now()
	code, err := call(s.c, "game.combat.cast", &protocol.CombatCastRequest{
		SkillId: skill, TargetUid: target,
	}, &protocol.None{})
	if err != nil && code == 0 {
		code = -2
	}
	return code, time.Since(start)
}

// statSnap 是最近一次自己属性帧的纯数值拷贝，避免跨锁传 protobuf 指针。
type statSnap struct {
	Attack, MaxHp, Defense     int32
	CritRate, CritDamage       int32
	DamageReduction, Lifesteal int32
	HP                         int32
}

func (s *slot) self() statSnap {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.selfStat == nil {
		return statSnap{}
	}
	return statSnap{
		Attack: s.selfStat.Attack, MaxHp: s.selfStat.MaxHp, Defense: s.selfStat.Defense,
		CritRate: s.selfStat.CritRate, CritDamage: s.selfStat.CritDamage,
		DamageReduction: s.selfStat.DamageReduction, Lifesteal: s.selfStat.Lifesteal,
		HP: s.selfStat.Hp,
	}
}

// hp 返回最后一次属性帧里的当前生命。
func (s *slot) hp() int32 { return s.self().HP }

// maxHP 返回最后一次属性帧里的生命上限。
func (s *slot) maxHP() int32 { return s.self().MaxHp }

func (s *slot) taken() []hitRec {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]hitRec(nil), s.hitsTaken...)
}

func (s *slot) healList() []hitRec {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]hitRec(nil), s.heals...)
}

func (s *slot) resetHits() {
	s.mu.Lock()
	s.hitsTaken = nil
	s.heals = nil
	s.mu.Unlock()
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
			s.selfStat = st
		}
	}
	if f.Self != nil && s.selfStat == nil {
		s.selfStat = &protocol.CombatStat{
			Uid: s.uid, Hp: f.Self.Hp, MaxHp: f.Self.MaxHp, Attack: f.Self.Attack, Defense: f.Self.Defense,
			CritRate: f.Self.CritRate, CritDamage: f.Self.CritDamage, Penetrate: f.Self.Penetrate,
			DamageBonus: f.Self.DamageBonus, DamageReduction: f.Self.DamageReduction, Lifesteal: f.Self.Lifesteal,
		}
	}
	for _, h := range f.Hits {
		if h == nil || h.SkillId != 1 {
			continue
		}
		rec := hitRec{amount: h.Amount, crit: h.Crit, skill: h.SkillId}
		if h.TargetUid == s.uid && !h.Heal {
			s.hitsTaken = append(s.hitsTaken, rec)
		}
		if h.TargetUid == s.uid && h.Heal && h.SourceUid == s.uid {
			s.heals = append(s.heals, rec)
		}
	}
}

func enterArena(slots []*slot, tag string) bool {
	n := len(slots)
	if !enterMain(slots, tag) {
		return false
	}
	codes := make([]int, n)
	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var rsp protocol.SceneSwitchResponse
			code, err := call(slots[i].c, "game.player.switchScene", &protocol.SceneSwitchRequest{SceneId: 2}, &rsp)
			if err != nil || rsp.SceneId != 2 || rsp.Line != 1 {
				if code == 0 {
					code = -5
				}
				codes[i] = code
				return
			}
			slots[i].line = rsp.Line
		}(i)
	}
	wg.Wait()
	printStats("switch-arena", time.Since(start), codes, nil)
	return count(codes, 0) == n
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
			s := &slot{}
			slots[i] = s
			c := pomeloClient.New(pomeloClient.WithRequestTimeout(20 * time.Second))
			if err := c.ConnectToWS(addr, ""); err != nil {
				codes[i] = -1
				return
			}
			s.c = c
			c.On("onCombatFrame", s.onFrame)
			var issued protocol.IssueTokenResponse
			code, err := call(c, "gate.user.issueToken", &protocol.IssueTokenRequest{
				Nickname: fmt.Sprintf("%s%d-%d", tag, stamp, i),
				Password: "123456", DeviceId: "lt20",
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
				AccessToken: issued.AccessToken, ServerId: 10001, DeviceId: "lt20",
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
			var entered protocol.EnterGameResponse
			code, err = call(c, "game.player.enter", &protocol.EnterGameRequest{
				PlayerId: created.Player.PlayerId, SceneId: 1,
			}, &entered)
			if err != nil || entered.SceneId != 1 {
				if code == 0 {
					code = -5
				}
				codes[i] = code
			}
		}(i)
	}
	wg.Wait()
	printStats("enter", time.Since(start), codes, nil)
	return count(codes, 0) == n
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
