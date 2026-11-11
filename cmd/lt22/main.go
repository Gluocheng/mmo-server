// LT-22 全链路：2 人在演武场挂持续伤害。
// 目标穿布衣（防御 3、受伤减免 5），来源不穿暴击装备。
// 撕裂基数 4。减防后为 1，再乘 95% 仍至少为 1，且不暴击。
// 脱掉布衣后再挂一次，基数 4 原样跳伤。
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
	crit   bool
	heal   bool
}

type slot struct {
	c   *pomeloClient.Client
	uid int64

	mu      sync.Mutex
	defense int32
	reduce  int32
	gotStat bool
	hits    []hitRec
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
	fmt.Println("== 全链路 2 人挂持续伤害 ==")
	slots := make([]*slot, 2)
	if !enterArena(slots, "d22") {
		closeAll(slots)
		return false
	}
	defer closeAll(slots)
	attacker, target := slots[0], slots[1]
	if !wearRobe(target) {
		fmt.Println("穿布衣失败")
		return false
	}
	if !waitStat(target, 3, 5) {
		fmt.Printf("布衣属性未到 defense=%d reduction=%d\n", target.defense, target.reduce)
		return false
	}
	fmt.Printf("目标防御=%d 受伤减免=%d\n", target.defense, target.reduce)

	target.reset()
	code, lat := cast(attacker, 2, target.uid)
	fmt.Printf("范围斩 code=%d 耗时=%s\n", code, lat.Round(time.Millisecond))
	if code != 0 {
		return false
	}
	time.Sleep(1500 * time.Millisecond)
	dots := target.dots()
	fmt.Printf("穿布衣跳伤 %+v\n", dots)
	if !oneDot(dots, 1, false) {
		return false
	}
	if n := attacker.dotHeals(); n != 0 {
		fmt.Printf("来源没有吸血却出现 %d 条治疗\n", n)
		return false
	}

	code, lat = unwear(target, 4)
	fmt.Printf("脱布衣 code=%d 耗时=%s\n", code, lat.Round(time.Millisecond))
	if code != 0 || !waitStat(target, 0, 0) {
		fmt.Printf("脱下后 defense=%d reduction=%d\n", target.defense, target.reduce)
		return false
	}
	time.Sleep(2 * time.Second)
	target.reset()
	code, lat = cast(attacker, 2, target.uid)
	fmt.Printf("再放范围斩 code=%d 耗时=%s\n", code, lat.Round(time.Millisecond))
	if code != 0 {
		return false
	}
	time.Sleep(1500 * time.Millisecond)
	dots = target.dots()
	fmt.Printf("无防具跳伤 %+v\n", dots)
	if !oneDot(dots, 4, false) {
		return false
	}
	fmt.Println("全链路通过")
	return true
}

func oneDot(dots []hitRec, amount int32, crit bool) bool {
	n := 0
	for _, h := range dots {
		if h.amount == amount && h.crit == crit {
			n++
		}
	}
	if n < 1 {
		fmt.Printf("没有跳伤 amount=%d crit=%v\n", amount, crit)
		return false
	}
	return true
}

func wearRobe(s *slot) bool {
	var rsp protocol.BagListResponse
	code, err := call(s.c, "game.bag.add", &protocol.BagAddRequest{ItemId: 2002, Count: 1}, &rsp)
	if err != nil || code != 0 {
		fmt.Printf("发布衣 code=%d err=%v\n", code, err)
		return false
	}
	bagSlot := int32(-1)
	for _, it := range rsp.Items {
		if it != nil && it.ItemId == 2002 {
			bagSlot = it.Slot
			break
		}
	}
	if bagSlot < 0 {
		fmt.Println("背包没有布衣")
		return false
	}
	code, err = call(s.c, "game.equip.wear", &protocol.EquipWearRequest{
		BagType: 4, Slot: bagSlot, TargetSlot: 4,
	}, &protocol.EquipState{})
	if err != nil || code != 0 {
		fmt.Printf("穿布衣 code=%d err=%v\n", code, err)
		return false
	}
	return true
}

func unwear(s *slot, slotID int32) (int, time.Duration) {
	start := time.Now()
	code, err := call(s.c, "game.equip.unwear", &protocol.EquipUnwearRequest{SlotId: slotID}, &protocol.EquipState{})
	if err != nil && code == 0 {
		code = -2
	}
	return code, time.Since(start)
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

func waitStat(s *slot, defense, reduction int32) bool {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		ok := s.gotStat && s.defense == defense && s.reduce == reduction
		s.mu.Unlock()
		if ok {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
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

func (s *slot) dotHeals() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, h := range s.hits {
		if h.buff == 1 && h.heal {
			n++
		}
	}
	return n
}

func (s *slot) reset() {
	s.mu.Lock()
	s.hits = nil
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
			s.defense = st.Defense
			s.reduce = st.DamageReduction
			s.gotStat = true
		}
	}
	if f.Self != nil {
		s.defense = f.Self.Defense
		s.reduce = f.Self.DamageReduction
		s.gotStat = true
	}
	for _, h := range f.Hits {
		if h == nil {
			continue
		}
		if h.TargetUid != s.uid && !(h.Heal && h.SourceUid == s.uid) {
			continue
		}
		s.hits = append(s.hits, hitRec{
			buff: h.BuffId, skill: h.SkillId, amount: h.Amount, crit: h.Crit, heal: h.Heal,
		})
	}
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
	printStats("switch-arena", time.Since(start), codes, nil)
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
			var issued protocol.IssueTokenResponse
			code, err := call(c, "gate.user.issueToken", &protocol.IssueTokenRequest{
				Nickname: fmt.Sprintf("%s%d-%d", tag, stamp, i),
				Password: "123456", DeviceId: "lt22",
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
				AccessToken: issued.AccessToken, ServerId: 10001, DeviceId: "lt22",
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
	fmt.Printf("%s 成功=%d 失败=%d 墙钟=%s 返回码 %s\n", name, okN, len(codes)-okN, wall.Round(time.Millisecond), formatCounts(codes))
	_ = lats
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
