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
	loadN  = 50
	wildID = int32(3)
	wolfX  = float32(15)
	wolfZ  = float32(10)
)

var addr string

type slot struct {
	c      *pomeloClient.Client
	uid    int64
	wolf   int64
	mu     sync.Mutex
	moves  []int64
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
	fmt.Println("== 全链路 2 人靠近野狼，被追被打，再点名 ==")
	slots := make([]*slot, 2)
	if !enterMain(slots, "fc") || !switchWild(slots) {
		closeAll(slots)
		return false
	}
	ok := true
	for _, s := range slots {
		if s.wolf < 1 {
			fmt.Printf("uid=%d 附近没有野狼\n", s.uid)
			ok = false
		}
	}
	if !ok {
		closeAll(slots)
		return false
	}
	for _, s := range slots {
		s.clear()
	}
	codes, lats, wall := moveAll(slots, wolfX, 0, wolfZ)
	printStats("move", wall, codes, lats)
	ok = count(codes, 0) == len(slots)
	chased, bitten := waitWolf(slots, 4*time.Second)
	fmt.Printf("看到追击=%d 看到还手=%d\n", chased, bitten)
	ok = ok && chased == len(slots) && bitten == len(slots)
	for _, s := range slots {
		s.clear()
	}
	codes, lats, wall = castAll(slots)
	printStats("cast", wall, codes, lats)
	ok = ok && count(codes, 0) == len(slots)
	hit := waitHit(slots, 2*time.Second)
	fmt.Printf("点名打中=%d\n", hit)
	ok = ok && hit == len(slots)
	closeAll(slots)
	time.Sleep(2 * time.Second)
	if ok {
		fmt.Println("全链路通过")
	}
	return ok
}

func load() bool {
	fmt.Printf("== 压测 %d 人靠近野狼 ==\n", loadN)
	slots := make([]*slot, loadN)
	if !enterMain(slots, "ld") || !switchWild(slots) {
		closeAll(slots)
		return false
	}
	seen := 0
	for _, s := range slots {
		if s.wolf > 0 {
			seen++
		}
		s.clear()
	}
	fmt.Printf("附近有野狼=%d\n", seen)
	ok := seen == loadN
	codes, lats, wall := moveAll(slots, wolfX, 0, wolfZ)
	printStats("move", wall, codes, lats)
	ok = ok && count(codes, 0) == loadN
	chased, bitten := waitWolf(slots, 4*time.Second)
	fmt.Printf("看到追击=%d 看到还手=%d\n", chased, bitten)
	ok = ok && chased == loadN && bitten == loadN
	for _, s := range slots {
		s.clear()
	}
	codes, lats, wall = castAll(slots)
	printStats("cast", wall, codes, lats)
	ok = ok && count(codes, 0) == loadN
	hit := waitHit(slots, 2*time.Second)
	fmt.Printf("点名打中=%d\n", hit)
	ok = ok && hit == loadN
	closeAll(slots)
	if ok {
		fmt.Println("压测通过")
	}
	return ok
}

func moveAll(slots []*slot, x, y, z float32) ([]int, []time.Duration, time.Duration) {
	n := len(slots)
	codes := make([]int, n)
	lats := make([]time.Duration, n)
	var wg sync.WaitGroup
	start := time.Now()
	for i := range slots {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			t := time.Now()
			code, err := call(slots[i].c, "game.player.move", &protocol.MoveRequest{X: x, Y: y, Z: z}, &protocol.None{})
			lats[i] = time.Since(t)
			if err != nil && code == 0 {
				code = -2
			}
			codes[i] = code
		}(i)
	}
	wg.Wait()
	return codes, lats, time.Since(start)
}

func castAll(slots []*slot) ([]int, []time.Duration, time.Duration) {
	n := len(slots)
	codes := make([]int, n)
	lats := make([]time.Duration, n)
	var wg sync.WaitGroup
	start := time.Now()
	for i := range slots {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			t := time.Now()
			code, err := call(slots[i].c, "game.combat.cast", &protocol.CombatCastRequest{SkillId: 1, TargetUid: slots[i].wolf}, &protocol.None{})
			lats[i] = time.Since(t)
			if err != nil && code == 0 {
				code = -2
			}
			codes[i] = code
		}(i)
	}
	wg.Wait()
	return codes, lats, time.Since(start)
}

func waitWolf(slots []*slot, d time.Duration) (chased, bitten int) {
	deadline := time.Now().Add(d)
	for {
		chased, bitten = 0, 0
		for _, s := range slots {
			if s.sawMove() {
				chased++
			}
			if s.sawBite() {
				bitten++
			}
		}
		if (chased == len(slots) && bitten == len(slots)) || time.Now().After(deadline) {
			return chased, bitten
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func waitHit(slots []*slot, d time.Duration) int {
	deadline := time.Now().Add(d)
	for {
		n := 0
		for _, s := range slots {
			if s.sawPlayerHit() {
				n++
			}
		}
		if n == len(slots) || time.Now().After(deadline) {
			return n
		}
		time.Sleep(20 * time.Millisecond)
	}
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
			c.On("onMove", func(msg *pomeloMessage.Message) {
				var mv protocol.MoveBroadcast
				if len(msg.Data) == 0 || c.Serializer().Unmarshal(msg.Data, &mv) != nil || mv.Uid < 1 {
					return
				}
				s.mu.Lock()
				s.moves = append(s.moves, mv.Uid)
				s.mu.Unlock()
			})
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
				DeviceId: "lt10",
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
				AccessToken: issued.AccessToken, ServerId: 10001, DeviceId: "lt10",
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
				Name: fmt.Sprintf("w%d-%d", stamp%1000000, i),
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

func switchWild(slots []*slot) bool {
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
			code, err := call(slots[i].c, "game.player.switchScene", &protocol.SceneSwitchRequest{SceneId: wildID}, &rsp)
			lats[i] = time.Since(t)
			if err != nil || rsp.SceneId != wildID || rsp.Line != 1 {
				if code == 0 {
					code = -5
				}
				codes[i] = code
				return
			}
			slots[i].wolf = wolfOf(rsp.Nearby)
			if slots[i].wolf < 1 {
				codes[i] = -8
				return
			}
			codes[i] = 0
		}(i)
	}
	wg.Wait()
	printStats("switch-wild", time.Since(start), codes, lats)
	return count(codes, 0) == n
}

func wolfOf(nearby []*protocol.SceneActor) int64 {
	for _, n := range nearby {
		if n != nil && n.ActorType == 1 && n.ConfigId == 1 {
			return n.Uid
		}
	}
	return 0
}

func (s *slot) sawMove() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range s.moves {
		if id == s.wolf {
			return true
		}
	}
	return false
}

func (s *slot) sawBite() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, f := range s.frames {
		for _, h := range f.GetHits() {
			if h != nil && h.SkillId == 0 && h.SourceUid == s.wolf && h.Amount > 0 {
				return true
			}
		}
	}
	return false
}

func (s *slot) sawPlayerHit() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, f := range s.frames {
		for _, h := range f.GetHits() {
			if h != nil && h.SkillId == 1 && h.TargetUid == s.wolf && h.Amount > 0 {
				return true
			}
		}
	}
	return false
}

func (s *slot) clear() {
	s.mu.Lock()
	s.moves = nil
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
	var timed []time.Duration
	major := 0
	best := -1
	seen := map[int]int{}
	for _, c := range codes {
		seen[c]++
		if seen[c] > best {
			best = seen[c]
			major = c
		}
	}
	for i, c := range codes {
		if c == major && i < len(lats) && lats[i] > 0 {
			timed = append(timed, lats[i])
		}
	}
	sort.Slice(timed, func(i, j int) bool { return timed[i] < timed[j] })
	if wall > 0 {
		fmt.Printf("%s 墙钟=%s 返回码 %s\n", name, wall.Round(time.Millisecond), formatCounts(codes))
	} else {
		fmt.Printf("%s 返回码 %s\n", name, formatCounts(codes))
	}
	if len(timed) > 0 {
		fmt.Printf("耗时 p50=%s p95=%s max=%s\n", pct(timed, 50), pct(timed, 95), timed[len(timed)-1].Round(time.Millisecond))
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
