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
	c    *pomeloClient.Client
	uid  int64
	line int32
	mu   sync.Mutex
	got  map[int64]string
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
	fmt.Println("== 全链路 2 人 ==")
	slots := make([]*slot, 2)
	if !prepare(slots, "fc") {
		closeAll(slots)
		return false
	}
	for _, s := range slots {
		s.reset()
	}
	ok := true
	for i, s := range slots {
		text := fmt.Sprintf("hello-%d", i)
		code, lat := send(s, text)
		fmt.Printf("发送方 %d code=%d 耗时=%s\n", i, code, lat.Round(time.Millisecond))
		if code != 0 {
			ok = false
		}
	}
	time.Sleep(800 * time.Millisecond)
	for i, s := range slots {
		other := slots[1-i]
		s.mu.Lock()
		got := s.got[other.uid]
		s.mu.Unlock()
		want := fmt.Sprintf("hello-%d", 1-i)
		fmt.Printf("接收方 %d 收到 %q\n", i, got)
		if got != want {
			ok = false
		}
	}
	closeAll(slots)
	if ok {
		fmt.Println("全链路通过")
	}
	return ok
}

func load() bool {
	fmt.Printf("== 压测 %d 人同时发言 ==\n", loadN)
	slots := make([]*slot, loadN)
	if !prepare(slots, "ld") {
		closeAll(slots)
		return false
	}
	for _, s := range slots {
		s.reset()
	}
	codes := make([]int, loadN)
	lats := make([]time.Duration, loadN)
	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < loadN; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes[i], lats[i] = send(slots[i], fmt.Sprintf("m-%d", i))
		}(i)
	}
	wg.Wait()
	printStats("send", time.Since(start), codes, lats)
	time.Sleep(1500 * time.Millisecond)
	missing := 0
	for i, s := range slots {
		s.mu.Lock()
		for j, other := range slots {
			if i == j {
				continue
			}
			if s.got[other.uid] != fmt.Sprintf("m-%d", j) {
				missing++
			}
		}
		s.mu.Unlock()
	}
	fmt.Printf("缺失广播=%d 期望每人收到 %d 条\n", missing, loadN-1)
	closeAll(slots)
	return count(codes, 0) == loadN && missing == 0
}

func prepare(slots []*slot, tag string) bool {
	n := len(slots)
	codes := make([]int, n)
	stamp := time.Now().UnixNano()
	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s := &slot{got: map[int64]string{}}
			slots[i] = s
			c := pomeloClient.New(pomeloClient.WithRequestTimeout(20 * time.Second))
			if err := c.ConnectToWS(addr, ""); err != nil {
				codes[i] = -1
				return
			}
			s.c = c
			c.On("onChat", func(msg *pomeloMessage.Message) {
				var b protocol.ChatBroadcast
				if len(msg.Data) == 0 {
					return
				}
				if err := c.Serializer().Unmarshal(msg.Data, &b); err != nil || b.Uid < 1 {
					return
				}
				s.mu.Lock()
				s.got[b.Uid] = b.Text
				s.mu.Unlock()
			})
			var issued protocol.IssueTokenResponse
			code, err := call(c, "gate.user.issueToken", &protocol.IssueTokenRequest{
				Nickname: fmt.Sprintf("%s%d-%d", tag, stamp, i),
				Password: "123456",
				DeviceId: "lt05",
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
				AccessToken: issued.AccessToken, ServerId: 10001, DeviceId: "lt05",
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
	for _, s := range slots[1:] {
		if s.line != line {
			fmt.Printf("不在同一条线\n")
			return false
		}
	}
	fmt.Printf("主城 %d 线 ×%d\n", line, n)
	return true
}

func send(s *slot, text string) (int, time.Duration) {
	if s == nil || s.c == nil {
		return -1, 0
	}
	start := time.Now()
	code, err := call(s.c, "game.chat.send", &protocol.ChatSendRequest{Text: text}, &protocol.None{})
	if err != nil && code == 0 {
		code = -2
	}
	return code, time.Since(start)
}

func (s *slot) reset() {
	s.mu.Lock()
	s.got = map[int64]string{}
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
