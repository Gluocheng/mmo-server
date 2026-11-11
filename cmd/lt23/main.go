// LT-23 全链路：2 人在演武场验证躲闪。
// 目标穿软布鞋，躲闪率 2000。普攻有的躲开（伤害 0、不暴击、生命不变），
// 有的没躲开（掉 10、不暴击）。攻击方不穿吸血装备，躲开和命中都不该出现治疗。
// 范围斩附带的撕裂照常跳 4，且不标躲闪。
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

var addr string

type hitRec struct {
	buff   int32
	skill  int32
	amount int32
	hp     int32
	crit   bool
	heal   bool
	dodge  bool
	dead   bool
}

type slot struct {
	c   *pomeloClient.Client
	uid int64

	mu        sync.Mutex
	dodgeRate int32
	gotDodge  bool
	hp        int32
	gotHP     bool
	hits      []hitRec
}

func main() {
	flag.StringVar(&addr, "addr", "gate-entry:10100", "gate entry host:port")
	flag.Parse()
	if err := waitReady(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if !fullChain() {
		os.Exit(1)
	}
}

func fullChain() bool {
	fmt.Println("== 全链路 2 人躲闪 ==")
	slots := make([]*slot, 2)
	if !enterArena(slots, "d23") {
		closeAll(slots)
		return false
	}
	defer closeAll(slots)
	attacker, target := slots[0], slots[1]
	if !wearShoes(target) {
		fmt.Println("穿软布鞋失败")
		return false
	}
	if !waitDodge(target, 2000) {
		fmt.Printf("软布鞋躲闪率未到 dodge=%d got=%v\n", target.dodgeRate, target.gotDodge)
		return false
	}
	fmt.Printf("目标躲闪率=%d\n", target.dodgeRate)
	if !addPotion(target, 10) {
		fmt.Println("发药水失败")
		return false
	}

	var lats []time.Duration
	dodges, hits := 0, 0
	prev := target.currentHP()
	for i := 0; i < 30 && (dodges == 0 || hits == 0); i++ {
		if prev > 0 && prev <= 30 {
			if !drink(target) || !target.waitHPAbove(prev, 3*time.Second) {
				fmt.Printf("喝药失败，当前生命=%d\n", target.currentHP())
				return false
			}
			prev = target.currentHP()
			fmt.Printf("喝药后生命=%d\n", prev)
		}
		before := len(target.skillHits(1))
		code, lat := cast(attacker, 1, target.uid)
		lats = append(lats, lat)
		fmt.Printf("普攻#%d code=%d 耗时=%s\n", i+1, code, lat.Round(time.Millisecond))
		if code != 0 {
			return false
		}
		h, ok := target.waitSkillHit(1, before, 2*time.Second)
		if !ok {
			fmt.Println("没有收到普攻命中")
			return false
		}
		if h.dead {
			fmt.Printf("目标被打死 %+v\n", h)
			return false
		}
		if h.dodge {
			if h.amount != 0 || h.crit || (prev > 0 && h.hp != prev) {
				fmt.Printf("躲开结算不对 prev=%d %+v\n", prev, h)
				return false
			}
			dodges++
		} else {
			if h.amount != 10 || h.crit || (prev > 0 && h.hp != prev-10) {
				fmt.Printf("命中结算不对 prev=%d %+v\n", prev, h)
				return false
			}
			hits++
		}
		prev = h.hp
		if n := attacker.skillHeals(1); n != 0 {
			fmt.Printf("普攻出现 %d 条治疗\n", n)
			return false
		}
		time.Sleep(1100 * time.Millisecond)
	}
	fmt.Printf("普攻躲开=%d 命中=%d\n", dodges, hits)
	if dodges == 0 || hits == 0 {
		fmt.Println("30 次里没有同时看到躲开和命中")
		return false
	}

	if prev <= 20 {
		if !drink(target) || !target.waitHPAbove(prev, 3*time.Second) {
			fmt.Printf("跳伤前喝药失败，当前生命=%d\n", target.currentHP())
			return false
		}
		fmt.Printf("跳伤前生命=%d\n", target.currentHP())
	}
	target.resetHits()
	code, lat := cast(attacker, 2, target.uid)
	lats = append(lats, lat)
	fmt.Printf("范围斩 code=%d 耗时=%s\n", code, lat.Round(time.Millisecond))
	if code != 0 {
		return false
	}
	time.Sleep(1500 * time.Millisecond)
	dots := target.dots()
	fmt.Printf("撕裂 %+v\n", dots)
	if !dotsOK(dots) {
		return false
	}
	p50, p95 := percentiles(lats)
	fmt.Printf("出手 %d 次 p50=%s p95=%s\n", len(lats), p50.Round(time.Millisecond), p95.Round(time.Millisecond))
	fmt.Println("全链路通过")
	return true
}

func dotsOK(dots []hitRec) bool {
	n := 0
	for _, h := range dots {
		if h.dodge || h.crit || h.amount != 4 {
			fmt.Printf("跳伤不符合 %+v\n", h)
			return false
		}
		n++
	}
	if n < 1 {
		fmt.Println("没有撕裂跳伤")
		return false
	}
	return true
}

func wearShoes(s *slot) bool {
	var rsp protocol.BagListResponse
	code, err := call(s.c, "game.bag.add", &protocol.BagAddRequest{ItemId: 2005, Count: 1}, &rsp)
	if err != nil || code != 0 {
		fmt.Printf("发软布鞋 code=%d err=%v\n", code, err)
		return false
	}
	bagSlot := int32(-1)
	for _, it := range rsp.Items {
		if it != nil && it.ItemId == 2005 {
			bagSlot = it.Slot
			break
		}
	}
	if bagSlot < 0 {
		fmt.Println("背包没有软布鞋")
		return false
	}
	code, err = call(s.c, "game.equip.wear", &protocol.EquipWearRequest{
		BagType: 4, Slot: bagSlot, TargetSlot: 8,
	}, &protocol.EquipState{})
	if err != nil || code != 0 {
		fmt.Printf("穿软布鞋 code=%d err=%v\n", code, err)
		return false
	}
	return true
}

func addPotion(s *slot, n int32) bool {
	code, err := call(s.c, "game.bag.add", &protocol.BagAddRequest{ItemId: 1001, Count: n}, &protocol.BagListResponse{})
	if err != nil || code != 0 {
		fmt.Printf("发药水 code=%d err=%v\n", code, err)
		return false
	}
	return true
}

func drink(s *slot) bool {
	var rsp protocol.BagListResponse
	code, err := call(s.c, "game.bag.list", &protocol.BagListRequest{BagType: 2}, &rsp)
	if err != nil || code != 0 {
		fmt.Printf("查药水 code=%d err=%v\n", code, err)
		return false
	}
	slotID := int32(-1)
	for _, it := range rsp.Items {
		if it != nil && it.ItemId == 1001 && it.Count > 0 {
			slotID = it.Slot
			break
		}
	}
	if slotID < 0 {
		fmt.Println("没有药水")
		return false
	}
	code, err = call(s.c, "game.bag.use", &protocol.BagUseRequest{BagType: 2, Slot: slotID}, &protocol.BagListResponse{})
	if err != nil || code != 0 {
		fmt.Printf("喝药 code=%d err=%v\n", code, err)
		return false
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

func waitDodge(s *slot, want int32) bool {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		ok := s.gotDodge && s.dodgeRate == want
		s.mu.Unlock()
		if ok {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func (s *slot) currentHP() int32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hp
}

func (s *slot) waitHPAbove(hp int32, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if s.currentHP() > hp {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

func (s *slot) skillHits(skill int32) []hitRec {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []hitRec
	for _, h := range s.hits {
		if h.skill == skill && !h.heal && h.buff == 0 {
			out = append(out, h)
		}
	}
	return out
}

func (s *slot) waitSkillHit(skill int32, before int, d time.Duration) (hitRec, bool) {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		got := s.skillHits(skill)
		if len(got) > before {
			return got[before], true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return hitRec{}, false
}

func (s *slot) dots() []hitRec {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []hitRec
	for _, h := range s.hits {
		if h.buff == 1 && !h.heal {
			out = append(out, h)
		}
	}
	return out
}

func (s *slot) skillHeals(skill int32) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, h := range s.hits {
		if h.skill == skill && h.heal {
			n++
		}
	}
	return n
}

func (s *slot) resetHits() {
	s.mu.Lock()
	s.hits = nil
	s.mu.Unlock()
}

func (s *slot) noteStat(uid int64, hp, dodge int32, hasDodge bool) {
	if uid != 0 && uid != s.uid {
		return
	}
	if hp > 0 {
		s.hp = hp
		s.gotHP = true
	}
	if hasDodge {
		s.dodgeRate = dodge
		s.gotDodge = true
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
			s.noteStat(st.Uid, st.Hp, st.DodgeRate, st.DodgeRate > 0 || s.gotDodge)
		}
	}
	if f.Self != nil {
		s.noteStat(s.uid, f.Self.Hp, f.Self.DodgeRate, f.Self.DodgeRate > 0 || s.gotDodge)
	}
	for _, h := range f.Hits {
		if h == nil {
			continue
		}
		if h.TargetUid != s.uid && !(h.Heal && h.SourceUid == s.uid) {
			continue
		}
		s.hits = append(s.hits, hitRec{
			buff: h.BuffId, skill: h.SkillId, amount: h.Amount, hp: h.TargetHp,
			crit: h.Crit, heal: h.Heal, dodge: h.Dodge, dead: h.Dead,
		})
		if h.TargetUid == s.uid {
			s.hp = h.TargetHp
			s.gotHP = true
		}
	}
}

func (s *slot) onSnap(msg *pomeloMessage.Message) {
	if len(msg.Data) == 0 {
		return
	}
	var snap protocol.CombatSnapshot
	if s.c.Serializer().Unmarshal(msg.Data, &snap) != nil || snap.Uid != s.uid {
		return
	}
	s.mu.Lock()
	s.noteStat(snap.Uid, snap.Hp, snap.DodgeRate, true)
	s.mu.Unlock()
}

func enterArena(slots []*slot, tag string) bool {
	if !enterMain(slots, tag) {
		return false
	}
	lines := make([]int32, len(slots))
	codes := make([]int, len(slots))
	var wg sync.WaitGroup
	start := time.Now()
	for i := range slots {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var rsp protocol.SceneSwitchResponse
			code, err := call(slots[i].c, "game.player.switchScene", &protocol.SceneSwitchRequest{SceneId: 2}, &rsp)
			if err != nil || rsp.SceneId != 2 || rsp.Line < 1 {
				if code == 0 {
					code = -5
				}
				codes[i] = code
				return
			}
			lines[i] = rsp.Line
		}(i)
	}
	wg.Wait()
	printStats("switch-arena", time.Since(start), codes)
	if count(codes, 0) != len(slots) {
		return false
	}
	if lines[0] != lines[1] {
		fmt.Printf("不在同一条线 %v\n", lines)
		return false
	}
	fmt.Printf("演武场 %d 线\n", lines[0])
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
			s := &slot{}
			slots[i] = s
			c := pomeloClient.New(pomeloClient.WithRequestTimeout(20 * time.Second))
			if err := c.ConnectToWS(addr, ""); err != nil {
				codes[i] = -1
				return
			}
			s.c = c
			c.On("onCombatFrame", s.onFrame)
			c.On("onCombatSnapshot", s.onSnap)
			var issued protocol.IssueTokenResponse
			code, err := call(c, "gate.user.issueToken", &protocol.IssueTokenRequest{
				Nickname: fmt.Sprintf("%s%d-%d", tag, stamp, i),
				Password: "123456", DeviceId: "lt23",
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
				AccessToken: issued.AccessToken, ServerId: 10001, DeviceId: "lt23",
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
	printStats("enter", time.Since(start), codes)
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

func printStats(name string, wall time.Duration, codes []int) {
	okN := count(codes, 0)
	fmt.Printf("%s 成功=%d 失败=%d 墙钟=%s 返回码 %s\n", name, okN, len(codes)-okN, wall.Round(time.Millisecond), formatCounts(codes))
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

func percentiles(lats []time.Duration) (time.Duration, time.Duration) {
	if len(lats) == 0 {
		return 0, 0
	}
	cp := append([]time.Duration(nil), lats...)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	p50 := cp[(len(cp)-1)*50/100]
	p95 := cp[(len(cp)-1)*95/100]
	return p50, p95
}
