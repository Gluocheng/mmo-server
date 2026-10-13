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
	loadN    = 50
	potionID = int32(1001)
	bagType  = int32(2)
	arenaID  = int32(2)
)

var addr string

type slot struct {
	c      *pomeloClient.Client
	uid    int64
	potion int32
	count  int32
	hurtHP int32
	mu     sync.Mutex
	frames []*protocol.CombatFrame
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
	fmt.Println("== 全链路 2 人打掉血后喝药 ==")
	slots := make([]*slot, 2)
	if !enterMain(slots, "fc") || !switchArena(slots) {
		closeAll(slots)
		return false
	}
	ok := damageEachOther(slots)
	for _, s := range slots {
		code, lat := addPotion(s, 2)
		fmt.Printf("uid=%d 发药 code=%d 数量=%d 耗时=%s\n", s.uid, code, s.count, lat.Round(time.Millisecond))
		ok = ok && code == 0 && s.count == 2
	}
	for _, s := range slots {
		s.hurtHP = s.selfHP()
		s.clearFrames()
		code, lat := usePotion(s)
		heal, hp := s.waitHeal(2 * time.Second)
		fmt.Printf("uid=%d 喝药 code=%d 剩余=%d 耗时=%s 伤后生命=%d 治疗=%d 治疗后=%d\n",
			s.uid, code, s.count, lat.Round(time.Millisecond), s.hurtHP, heal, hp)
		ok = ok && code == 0 && s.count == 1 && heal > 0 && hp == s.hurtHP+heal
	}
	closeAll(slots)
	time.Sleep(2 * time.Second)
	if ok {
		fmt.Println("全链路通过")
	}
	return ok
}

func load() bool {
	fmt.Printf("== 压测 %d 人同时喝药 ==\n", loadN)
	slots := make([]*slot, loadN)
	if !enterMain(slots, "ld") || !switchArena(slots) {
		closeAll(slots)
		return false
	}
	ok := damageEachOther(slots)
	codes := make([]int, loadN)
	lats := make([]time.Duration, loadN)
	// 同时发药会在背包插入上互相死锁。药水先逐个发好，压测的是同时喝。
	start := time.Now()
	for i := range slots {
		code, lat := addPotion(slots[i], 2)
		codes[i] = code
		lats[i] = lat
	}
	printStats("add", time.Since(start), codes, lats)
	ok = ok && count(codes, 0) == loadN
	for _, s := range slots {
		s.hurtHP = s.selfHP()
		if s.count != 2 || s.hurtHP < 1 || s.hurtHP >= 100 {
			ok = false
		}
		s.clearFrames()
	}
	var wg sync.WaitGroup
	start = time.Now()
	for i := range slots {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			code, lat := usePotion(slots[i])
			codes[i] = code
			lats[i] = lat
		}(i)
	}
	wg.Wait()
	printStats("use", time.Since(start), codes, lats)
	ok = ok && count(codes, 0) == loadN
	heals := make([]int32, loadN)
	hps := make([]int32, loadN)
	for i := range slots {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			heals[i], hps[i] = slots[i].waitHeal(2 * time.Second)
		}(i)
	}
	wg.Wait()
	healed := 0
	kept := 0
	for i, s := range slots {
		if s.count == 1 {
			kept++
		}
		if heals[i] > 0 && hps[i] == s.hurtHP+heals[i] && hps[i] <= 100 {
			healed++
		} else {
			fmt.Printf("uid=%d 治疗异常 伤后=%d 治疗=%d 治疗后=%d\n", s.uid, s.hurtHP, heals[i], hps[i])
		}
	}
	fmt.Printf("只扣1个=%d 下一跳治疗=%d\n", kept, healed)
	ok = ok && kept == loadN && healed == loadN
	closeAll(slots)
	if ok {
		fmt.Println("压测通过")
	}
	return ok
}

// damageEachOther 每人点名下一个人，等自己的生命被打下来。
func damageEachOther(slots []*slot) bool {
	n := len(slots)
	codes := make([]int, n)
	lats := make([]time.Duration, n)
	var wg sync.WaitGroup
	start := time.Now()
	for i := range slots {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			target := slots[(i+1)%n].uid
			t := time.Now()
			code, err := call(slots[i].c, "game.combat.cast", &protocol.CombatCastRequest{SkillId: 1, TargetUid: target}, &protocol.None{})
			lats[i] = time.Since(t)
			if err != nil && code == 0 {
				code = -2
			}
			codes[i] = code
		}(i)
	}
	wg.Wait()
	printStats("cast", time.Since(start), codes, lats)
	hurt := 0
	for _, s := range slots {
		deadline := time.Now().Add(1500 * time.Millisecond)
		for time.Now().Before(deadline) && s.selfHP() == 0 {
			time.Sleep(20 * time.Millisecond)
		}
		if hp := s.selfHP(); hp > 0 && hp < 100 {
			hurt++
		} else {
			fmt.Printf("uid=%d 没有被打掉血 hp=%d\n", s.uid, hp)
		}
	}
	fmt.Printf("被打掉血=%d\n", hurt)
	return count(codes, 0) == n && hurt == n
}

func addPotion(s *slot, n int32) (int, time.Duration) {
	var rsp protocol.BagListResponse
	t := time.Now()
	code, err := call(s.c, "game.bag.add", &protocol.BagAddRequest{ItemId: potionID, Count: n}, &rsp)
	if err != nil && code == 0 {
		code = -2
	}
	slot, count := findPotion(&rsp)
	s.potion = slot
	s.count = count
	return code, time.Since(t)
}

func usePotion(s *slot) (int, time.Duration) {
	var rsp protocol.BagListResponse
	t := time.Now()
	code, err := call(s.c, "game.bag.use", &protocol.BagUseRequest{BagType: bagType, Slot: s.potion}, &rsp)
	if err != nil && code == 0 {
		code = -2
	}
	_, count := findPotion(&rsp)
	s.count = count
	return code, time.Since(t)
}

func findPotion(rsp *protocol.BagListResponse) (slot, count int32) {
	if rsp == nil {
		return 0, 0
	}
	for _, it := range rsp.Items {
		if it != nil && it.ItemId == potionID {
			return it.Slot, it.Count
		}
	}
	return 0, 0
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
			c.On("onCombatFrame", func(msg *pomeloMessage.Message) {
				var frame protocol.CombatFrame
				if len(msg.Data) == 0 || c.Serializer().Unmarshal(msg.Data, &frame) != nil {
					return
				}
				s.mu.Lock()
				s.frames = append(s.frames, &frame)
				s.mu.Unlock()
			})
			var issued protocol.IssueTokenResponse
			code, err := call(c, "gate.user.issueToken", &protocol.IssueTokenRequest{
				Nickname: fmt.Sprintf("%s%d-%d", tag, stamp, i),
				Password: "123456",
				DeviceId: "lt08",
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
				AccessToken: issued.AccessToken, ServerId: 10001, DeviceId: "lt08",
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
				Name: fmt.Sprintf("p%d-%d", stamp%1000000, i),
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
				PlayerId: created.Player.PlayerId,
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

func switchArena(slots []*slot) bool {
	n := len(slots)
	codes := make([]int, n)
	lats := make([]time.Duration, n)
	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var rsp protocol.SceneSwitchResponse
			t := time.Now()
			code, err := call(slots[i].c, "game.player.switchScene", &protocol.SceneSwitchRequest{SceneId: arenaID}, &rsp)
			lats[i] = time.Since(t)
			if err != nil || rsp.SceneId != arenaID || rsp.Line != 1 {
				if code == 0 {
					code = -5
				}
				codes[i] = code
				return
			}
			codes[i] = 0
		}(i)
	}
	wg.Wait()
	printStats("switch-arena", time.Since(start), codes, lats)
	return count(codes, 0) == n
}

// selfHP 返回自己被打中后的当前生命。还没被打中时是 0。
func (s *slot) selfHP() int32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	var hp int32
	for _, f := range s.frames {
		for _, h := range f.GetHits() {
			if h != nil && !h.Heal && h.TargetUid == s.uid && h.TargetHp > 0 {
				hp = h.TargetHp
			}
		}
	}
	return hp
}

// waitHeal 等待自己这一瓶药的治疗跳数。返回实际加量和加完后的生命。
func (s *slot) waitHeal(d time.Duration) (amount, hp int32) {
	deadline := time.Now().Add(d)
	for {
		s.mu.Lock()
		for _, f := range s.frames {
			for _, h := range f.GetHits() {
				if h != nil && h.Heal && h.BuffId == 2 && h.TargetUid == s.uid {
					amount, hp = h.Amount, h.TargetHp
				}
			}
		}
		s.mu.Unlock()
		if amount > 0 || time.Now().After(deadline) {
			return amount, hp
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (s *slot) clearFrames() {
	s.mu.Lock()
	s.frames = nil
	s.mu.Unlock()
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
