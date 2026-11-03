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
	"github.com/example/mmo-server/internal/protocol"
)

// 测当前 Docker 能同时挂住多少条已进场连接。一波一波加，失败就停在上一波。
var (
	addr string
	wave int
	maxN int
	hold time.Duration
)

type conn struct {
	c    *pomeloClient.Client
	line int32
}

func main() {
	flag.StringVar(&addr, "addr", "gate-entry:10100", "gate entry")
	flag.IntVar(&wave, "wave", 200, "each wave size")
	flag.IntVar(&maxN, "max", 1200, "stop after this many attempts")
	flag.DurationVar(&hold, "hold", 15*time.Second, "hold the last good set")
	flag.Parse()
	if err := waitReady(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	var live []conn
	stamp := time.Now().UnixNano()
	next := 0
	lastGood := 0
	for len(live)+wave <= maxN {
		fmt.Printf("\n== 加 %d 人，目标在线 %d ==\n", wave, len(live)+wave)
		added, codes := addWave(stamp, next, wave)
		next += wave
		printStats("enter", codes)
		live = append(live, added...)
		fmt.Printf("本波成功 %d，当前在线 %d，分线 %s\n", len(added), len(live), formatLines(live))
		if len(added)*100 < wave*95 {
			fmt.Printf("本波成功率低于 95%%，停在上一档 %d\n", lastGood)
			closeLive(added)
			live = live[:len(live)-len(added)]
			break
		}
		lastGood = len(live)
	}
	fmt.Printf("\n稳住 %d 人 %s\n", len(live), hold)
	time.Sleep(hold)
	fmt.Printf("HOLD_DONE online=%d\n", len(live))
	closeLive(live)
}

func addWave(stamp int64, from, n int) ([]conn, []int) {
	out := make([]conn, n)
	codes := make([]int, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c, line, code := enterOne(fmt.Sprintf("cap%d-%d", stamp, from+i), fmt.Sprintf("c%d-%d", stamp%1000000, from+i))
			codes[i] = code
			if code == 0 && c != nil {
				out[i] = conn{c: c, line: line}
			}
		}(i)
	}
	wg.Wait()
	var live []conn
	for _, item := range out {
		if item.c != nil {
			live = append(live, item)
		}
	}
	return live, codes
}

func enterOne(nick, name string) (*pomeloClient.Client, int32, int) {
	c := pomeloClient.New(pomeloClient.WithRequestTimeout(30 * time.Second))
	if err := c.ConnectToWS(addr, ""); err != nil {
		return nil, 0, -1
	}
	var issued protocol.IssueTokenResponse
	code, err := call(c, "gate.user.issueToken", &protocol.IssueTokenRequest{
		Nickname: nick, Password: "123456", DeviceId: "lt-cap",
	}, &issued)
	if err != nil || issued.AccessToken == "" {
		c.Disconnect()
		if code == 0 {
			code = -3
		}
		return nil, 0, code
	}
	var login protocol.TokenLoginResponse
	code, err = call(c, "gate.user.login", &protocol.TokenLoginRequest{
		AccessToken: issued.AccessToken, ServerId: 10001, DeviceId: "lt-cap",
	}, &login)
	if err != nil || login.Uid < 1 {
		c.Disconnect()
		if code == 0 {
			code = -3
		}
		return nil, 0, code
	}
	_, _ = call(c, "game.player.select", &protocol.None{}, &protocol.PlayerSelectResponse{})
	var created protocol.PlayerCreateResponse
	code, err = call(c, "game.player.create", &protocol.PlayerCreateRequest{Name: name}, &created)
	if err != nil || created.Player == nil {
		c.Disconnect()
		if code == 0 {
			code = -3
		}
		return nil, 0, code
	}
	var entered protocol.EnterGameResponse
	code, err = call(c, "game.player.enter", &protocol.EnterGameRequest{PlayerId: created.Player.PlayerId}, &entered)
	if err != nil || entered.SceneId != 1 {
		c.Disconnect()
		if code == 0 {
			code = -5
		}
		return nil, 0, code
	}
	return c, entered.Line, 0
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

func closeLive(items []conn) {
	for _, item := range items {
		if item.c != nil {
			item.c.Disconnect()
		}
	}
}

func printStats(name string, codes []int) {
	fmt.Printf("%s 返回码 %s\n", name, formatCounts(codes))
}

func formatLines(items []conn) string {
	counts := map[int32]int{}
	for _, item := range items {
		counts[item.line]++
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
