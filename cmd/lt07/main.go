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

type slot struct {
	c      *pomeloClient.Client
	uid    int64
	line   int32
	scene  int32
	mu     sync.Mutex
	frames []time.Time
	skills map[int32]int
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
	fmt.Println("== 全链路 2 人在演武场施法 ==")
	slots := make([]*slot, 2)
	if !enterArena(slots, "fc") {
		closeAll(slots)
		return false
	}
	for _, s := range slots {
		s.reset()
	}
	code, lat := cast(slots[0], 1, slots[1].uid)
	fmt.Printf("点名 code=%d 耗时=%s\n", code, lat.Round(time.Millisecond))
	time.Sleep(400 * time.Millisecond)
	ok := code == 0 && slots[1].sawSkill(1)
	fmt.Printf("对方收到点名=%v 帧数=%d\n", slots[1].sawSkill(1), slots[1].frameCount())
	code, lat = cast(slots[0], 2, 0)
	fmt.Printf("范围斩 code=%d 耗时=%s\n", code, lat.Round(time.Millisecond))
	time.Sleep(400 * time.Millisecond)
	ok = ok && code == 0 && slots[1].sawSkill(2)
	fmt.Printf("对方收到范围斩=%v 帧数=%d 最短帧间隔=%s\n", slots[1].sawSkill(2), slots[1].frameCount(), slots[1].minGap())
	if slots[1].tooClose(40 * time.Millisecond) {
		ok = false
		fmt.Println("全链路出现同一拍多条帧")
	}
	closeAll(slots)
	time.Sleep(1500 * time.Millisecond)
	if ok {
		fmt.Println("全链路通过")
	}
	return ok
}

func load() bool {
	fmt.Printf("== 压测 %d 人挤在演武场施法 ==\n", loadN)
	slots := make([]*slot, loadN)
	if !enterArena(slots, "ld") {
		closeAll(slots)
		return false
	}
	for _, s := range slots {
		s.reset()
	}
	fmt.Println("== 同时点名和范围斩 ==")
	codes := make([]int, loadN)
	lats := make([]time.Duration, loadN)
	start := time.Now()
	parallelCast(slots, codes, lats)
	printStats("cast", time.Since(start), codes, lats)
	firstOK := count(codes, 0) == loadN
	time.Sleep(1200 * time.Millisecond)
	fmt.Println("== 冷却结束后再点名一轮 ==")
	codes2 := make([]int, loadN)
	lats2 := make([]time.Duration, loadN)
	start = time.Now()
	var wg sync.WaitGroup
	for i := 0; i < loadN; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			target := slots[(i+1)%loadN].uid
			codes2[i], lats2[i] = cast(slots[i], 1, target)
		}(i)
	}
	wg.Wait()
	printStats("cast2", time.Since(start), codes2, lats2)
	time.Sleep(500 * time.Millisecond)
	silent, closeFrames := 0, 0
	var gaps []time.Duration
	for _, s := range slots {
		if s.frameCount() < 1 {
			silent++
		}
		if s.tooClose(40 * time.Millisecond) {
			closeFrames++
		}
		if g := s.minGap(); g > 0 {
			gaps = append(gaps, g)
		}
	}
	fmt.Printf("没收到帧=%d 帧间隔过近=%d\n", silent, closeFrames)
	if len(gaps) > 0 {
		sort.Slice(gaps, func(i, j int) bool { return gaps[i] < gaps[j] })
		fmt.Printf("帧间隔最小=%s 中位=%s\n", gaps[0].Round(time.Millisecond), gaps[len(gaps)/2].Round(time.Millisecond))
	}
	closeAll(slots)
	return firstOK && count(codes2, 0) == loadN && silent == 0 && closeFrames == 0
}

func parallelCast(slots []*slot, codes []int, lats []time.Duration) {
	var wg sync.WaitGroup
	for i := range slots {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%5 == 0 {
				codes[i], lats[i] = cast(slots[i], 2, 0)
				return
			}
			codes[i], lats[i] = cast(slots[i], 1, slots[(i+1)%len(slots)].uid)
		}(i)
	}
	wg.Wait()
}

func enterArena(slots []*slot, tag string) bool {
	n := len(slots)
	if !enterMain(slots, tag) {
		return false
	}
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
			code, err := call(slots[i].c, "game.player.switchScene", &protocol.SceneSwitchRequest{SceneId: 2}, &rsp)
			lats[i] = time.Since(t)
			if err != nil || rsp.SceneId != 2 || rsp.Line != 1 {
				if code == 0 {
					code = -5
				}
				codes[i] = code
				return
			}
			slots[i].scene = rsp.SceneId
			slots[i].line = rsp.Line
		}(i)
	}
	wg.Wait()
	printStats("switch-arena", time.Since(start), codes, lats)
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
			s := &slot{skills: map[int32]int{}}
			slots[i] = s
			c := pomeloClient.New(pomeloClient.WithRequestTimeout(20 * time.Second))
			if err := c.ConnectToWS(addr, ""); err != nil {
				codes[i] = -1
				return
			}
			s.c = c
			c.On("onCombatFrame", func(msg *pomeloMessage.Message) {
				var b protocol.CombatFrame
				if len(msg.Data) == 0 {
					return
				}
				if err := c.Serializer().Unmarshal(msg.Data, &b); err != nil {
					return
				}
				s.mu.Lock()
				s.frames = append(s.frames, time.Now())
				for _, h := range b.Hits {
					if h != nil && h.SkillId > 0 {
						s.skills[h.SkillId]++
					}
				}
				s.mu.Unlock()
			})
			var issued protocol.IssueTokenResponse
			code, err := call(c, "gate.user.issueToken", &protocol.IssueTokenRequest{
				Nickname: fmt.Sprintf("%s%d-%d", tag, stamp, i),
				Password: "123456",
				DeviceId: "lt07",
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
				AccessToken: issued.AccessToken, ServerId: 10001, DeviceId: "lt07",
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

func cast(s *slot, skill int32, target int64) (int, time.Duration) {
	if s == nil || s.c == nil {
		return -1, 0
	}
	start := time.Now()
	code, err := call(s.c, "game.combat.cast", &protocol.CombatCastRequest{
		SkillId: skill, TargetUid: target,
	}, &protocol.None{})
	if err != nil && code == 0 {
		code = -2
	}
	return code, time.Since(start)
}

func (s *slot) reset() {
	s.mu.Lock()
	s.frames = nil
	s.skills = map[int32]int{}
	s.mu.Unlock()
}

func (s *slot) sawSkill(id int32) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.skills[id] > 0
}

func (s *slot) frameCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.frames)
}

func (s *slot) minGap() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.frames) < 2 {
		return 0
	}
	cp := append([]time.Time(nil), s.frames...)
	sort.Slice(cp, func(i, j int) bool { return cp[i].Before(cp[j]) })
	min := cp[1].Sub(cp[0])
	for i := 2; i < len(cp); i++ {
		if d := cp[i].Sub(cp[i-1]); d < min {
			min = d
		}
	}
	return min
}

func (s *slot) tooClose(limit time.Duration) bool {
	g := s.minGap()
	return g > 0 && g < limit
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
