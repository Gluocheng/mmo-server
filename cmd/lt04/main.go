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
	n         = 50
	nearCount = 25
	aoiFarZ   = float32(80)
)

var addr string

type slot struct {
	c    *pomeloClient.Client
	uid  int64
	line int32
	mu   sync.Mutex
	got  map[int64]int
}

func main() {
	flag.StringVar(&addr, "addr", "gate-entry:10100", "gate entry host:port")
	flag.Parse()
	if err := waitReady(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	slots := make([]*slot, n)
	if !prepare(slots) {
		closeAll(slots)
		os.Exit(1)
	}
	if !position(slots) {
		closeAll(slots)
		os.Exit(1)
	}
	time.Sleep(500 * time.Millisecond)
	for _, s := range slots {
		s.reset()
	}
	fmt.Println("== 半径内的人移动一次 ==")
	start := time.Now()
	code, lat := moveTo(slots[0], 1, 0, 0)
	fmt.Printf("probe move code=%d 耗时=%s 墙钟=%s\n", code, lat.Round(time.Millisecond), time.Since(start).Round(time.Millisecond))
	time.Sleep(800 * time.Millisecond)
	nearHit, farHit := countGroup(slots, slots[0].uid)
	fmt.Printf("近处收到=%d/%d 远处收到=%d/%d\n", nearHit, nearCount-1, farHit, n-nearCount)
	aoiOK := code == 0 && nearHit == nearCount-1 && farHit == 0

	fmt.Println("== 同图持续移动 10 轮 ==")
	for _, s := range slots {
		s.reset()
	}
	var codes []int
	var lats []time.Duration
	var mu sync.Mutex
	wallStart := time.Now()
	for round := 0; round < 10; round++ {
		var wg sync.WaitGroup
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(i, round int) {
				defer wg.Done()
				x, z := float32(i%3), float32(round%3)
				if i >= nearCount {
					x, z = 0, aoiFarZ
				}
				code, lat := moveTo(slots[i], x, 0, z)
				mu.Lock()
				codes = append(codes, code)
				lats = append(lats, lat)
				mu.Unlock()
			}(i, round)
		}
		wg.Wait()
	}
	time.Sleep(800 * time.Millisecond)
	printStats("move", time.Since(wallStart), codes, lats)
	cross := crossGroup(slots)
	fmt.Printf("跨半径串线=%d\n", cross)
	closeAll(slots)
	if !aoiOK || count(codes, 0) != len(codes) || cross != 0 {
		os.Exit(1)
	}
}

func prepare(slots []*slot) bool {
	fmt.Printf("== 准备 %d 人进同一条主城线 ==\n", n)
	codes := make([]int, n)
	stamp := time.Now().UnixNano()
	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s := &slot{got: map[int64]int{}}
			slots[i] = s
			c := pomeloClient.New(pomeloClient.WithRequestTimeout(20 * time.Second))
			if err := c.ConnectToWS(addr, ""); err != nil {
				codes[i] = -1
				return
			}
			s.c = c
			c.On("onMove", func(msg *pomeloMessage.Message) {
				var b protocol.MoveBroadcast
				if len(msg.Data) == 0 {
					return
				}
				if err := c.Serializer().Unmarshal(msg.Data, &b); err != nil || b.Uid < 1 {
					return
				}
				s.mu.Lock()
				s.got[b.Uid]++
				s.mu.Unlock()
			})
			var issued protocol.IssueTokenResponse
			code, err := call(c, "gate.user.issueToken", &protocol.IssueTokenRequest{
				Nickname: fmt.Sprintf("m%d-%d", stamp, i),
				Password: "123456",
				DeviceId: "lt04",
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
				AccessToken: issued.AccessToken, ServerId: 10001, DeviceId: "lt04",
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
				Name: fmt.Sprintf("mv%d-%d", stamp%1000000, i),
			}, &created)
			if err != nil || created.Player == nil || created.Player.PlayerId < 1 {
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
	printStats("prepare", time.Since(start), codes, nil)
	if count(codes, 0) != n {
		return false
	}
	line := slots[0].line
	for _, s := range slots {
		if s.line != line {
			fmt.Printf("不在同一条线: %s\n", formatLines(slots))
			return false
		}
	}
	fmt.Printf("主城分线 %s\n", formatLines(slots))
	return true
}

func position(slots []*slot) bool {
	fmt.Printf("== 把 %d 人移到半径外 z=%.0f ==\n", n-nearCount, aoiFarZ)
	codes := make([]int, n-nearCount)
	lats := make([]time.Duration, n-nearCount)
	var wg sync.WaitGroup
	start := time.Now()
	for i := nearCount; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes[i-nearCount], lats[i-nearCount] = moveTo(slots[i], 0, 0, aoiFarZ)
		}(i)
	}
	wg.Wait()
	printStats("position", time.Since(start), codes, lats)
	return count(codes, 0) == len(codes)
}

func moveTo(s *slot, x, y, z float32) (int, time.Duration) {
	if s == nil || s.c == nil {
		return -1, 0
	}
	start := time.Now()
	code, err := call(s.c, "game.player.move", &protocol.MoveRequest{X: x, Y: y, Z: z}, &protocol.None{})
	if err != nil && code == 0 {
		code = -2
	}
	return code, time.Since(start)
}

func countGroup(slots []*slot, uid int64) (near, far int) {
	for i, s := range slots {
		if i == 0 || !s.saw(uid) {
			continue
		}
		if i < nearCount {
			near++
		} else {
			far++
		}
	}
	return near, far
}

func crossGroup(slots []*slot) int {
	nearUIDs := map[int64]struct{}{}
	farUIDs := map[int64]struct{}{}
	for i, s := range slots {
		if i < nearCount {
			nearUIDs[s.uid] = struct{}{}
		} else {
			farUIDs[s.uid] = struct{}{}
		}
	}
	bad := 0
	for i, s := range slots {
		s.mu.Lock()
		for uid := range s.got {
			if i < nearCount {
				if _, ok := farUIDs[uid]; ok {
					bad++
				}
			} else if _, ok := nearUIDs[uid]; ok {
				bad++
			}
		}
		s.mu.Unlock()
	}
	return bad
}

func (s *slot) reset() {
	s.mu.Lock()
	s.got = map[int64]int{}
	s.mu.Unlock()
}

func (s *slot) saw(uid int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.got[uid] > 0
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
	code, conv := strconv.Atoi(strings.TrimSpace(rest[:end]))
	if conv != nil {
		return -1
	}
	return code
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

func formatLines(slots []*slot) string {
	counts := map[int32]int{}
	for _, s := range slots {
		if s != nil && s.line > 0 {
			counts[s.line]++
		}
	}
	keys := make([]int, 0, len(counts))
	for k := range counts {
		keys = append(keys, int(k))
	}
	sort.Ints(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%d线×%d", k, counts[int32(k)]))
	}
	if len(parts) == 0 {
		return "无"
	}
	return strings.Join(parts, " ")
}
