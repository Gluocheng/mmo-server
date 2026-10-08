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

var addr string

type pair struct {
	access  string
	refresh string
}

func main() {
	flag.StringVar(&addr, "addr", "127.0.0.1:10100", "gate entry host:port")
	flag.Parse()
	if err := waitReady(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	failed := false
	for _, n := range []int{200, 1000} {
		fmt.Printf("\n######## %d 账号 ########\n", n)
		if !lt01(n) {
			failed = true
		}
		if !lt02(n) {
			failed = true
		}
		if !lt03(n) {
			failed = true
		}
	}
	if failed {
		os.Exit(1)
	}
}

func waitReady() error {
	deadline := time.Now().Add(90 * time.Second)
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

func lt01(n int) bool {
	fmt.Printf("== LT-01 同时签发 %d ==\n", n)
	codes := make([]int, n)
	lats := make([]time.Duration, n)
	stamp := time.Now().UnixNano()
	var sample error
	var once sync.Once
	start := time.Now()
	parallel(n, func(i int) {
		var rsp protocol.IssueTokenResponse
		code, lat, err := request("gate.user.issueToken", &protocol.IssueTokenRequest{
			Nickname: fmt.Sprintf("a%d-%d", stamp, i),
			Password: "123456",
			DeviceId: "lt-scale",
		}, &rsp)
		codes[i], lats[i] = code, lat
		if err != nil {
			once.Do(func() { sample = err })
		}
		if err == nil && (rsp.AccessToken == "" || rsp.RefreshToken == "") {
			codes[i] = -3
		}
	})
	if sample != nil && count(codes, 0) != n {
		fmt.Printf("失败样例: %v\n", sample)
	}
	printStats("issueToken", time.Since(start), codes, lats)
	return count(codes, 0) == n
}

func lt02(n int) bool {
	fmt.Printf("== LT-02 签发 %d 个号 ==\n", n)
	accounts := make([]pair, n)
	codes := make([]int, n)
	stamp := time.Now().UnixNano()
	start := time.Now()
	parallel(n, func(i int) {
		var rsp protocol.IssueTokenResponse
		code, _, err := request("gate.user.issueToken", &protocol.IssueTokenRequest{
			Nickname: fmt.Sprintf("b%d-%d", stamp, i),
			Password: "123456",
			DeviceId: "lt-scale",
		}, &rsp)
		codes[i] = code
		if err == nil && rsp.AccessToken != "" && rsp.RefreshToken != "" {
			accounts[i] = pair{rsp.AccessToken, rsp.RefreshToken}
		} else if code == 0 {
			codes[i] = -3
		}
	})
	printStats("prepare", time.Since(start), codes, nil)
	if count(codes, 0) != n {
		return false
	}

	fmt.Printf("== LT-02 同时登录 %d ==\n", n)
	codes = make([]int, n)
	lats := make([]time.Duration, n)
	start = time.Now()
	parallel(n, func(i int) {
		var rsp protocol.TokenLoginResponse
		code, lat, err := request("gate.user.login", &protocol.TokenLoginRequest{
			AccessToken: accounts[i].access, ServerId: 10001, DeviceId: "lt-scale",
		}, &rsp)
		lats[i] = lat
		if err == nil && rsp.Uid < 1 {
			code = -3
		}
		codes[i] = code
	})
	printStats("login", time.Since(start), codes, lats)
	loginOK := count(codes, 0) == n

	fmt.Printf("== LT-02 同时刷新 %d ==\n", n)
	refreshed := make([]pair, n)
	oldRefresh := make([]string, n)
	codes = make([]int, n)
	lats = make([]time.Duration, n)
	start = time.Now()
	parallel(n, func(i int) {
		oldRefresh[i] = accounts[i].refresh
		var rsp protocol.RefreshTokenResponse
		code, lat, err := request("gate.user.refreshToken", &protocol.RefreshTokenRequest{
			RefreshToken: accounts[i].refresh,
		}, &rsp)
		lats[i] = lat
		if err == nil && rsp.AccessToken != "" && rsp.RefreshToken != "" {
			refreshed[i] = pair{rsp.AccessToken, rsp.RefreshToken}
		} else if code == 0 {
			code = -3
		}
		codes[i] = code
	})
	printStats("refresh", time.Since(start), codes, lats)
	refreshOK := count(codes, 0) == n

	fmt.Printf("== LT-02 同时重放旧 refresh %d ==\n", n)
	codes = make([]int, n)
	start = time.Now()
	parallel(n, func(i int) {
		code, _, _ := request("gate.user.refreshToken", &protocol.RefreshTokenRequest{
			RefreshToken: oldRefresh[i],
		}, &protocol.RefreshTokenResponse{})
		codes[i] = code
	})
	fmt.Printf("replay 40013=%d 其他=%d 墙钟=%s 返回码 %s\n", count(codes, 40013), n-count(codes, 40013), time.Since(start).Round(time.Millisecond), formatCounts(codes))
	replayOK := count(codes, 40013) == n

	fmt.Printf("== LT-02 同一条连接登录后同时登出 %d ==\n", n)
	codes = make([]int, n)
	lats = make([]time.Duration, n)
	start = time.Now()
	parallel(n, func(i int) {
		codes[i], lats[i] = loginAndLogout(refreshed[i])
	})
	printStats("logout", time.Since(start), codes, lats)
	logoutOK := count(codes, 0) == n

	fmt.Printf("== LT-02 登出后再登录 %d ==\n", n)
	codes = make([]int, n)
	start = time.Now()
	parallel(n, func(i int) {
		var rsp protocol.TokenLoginResponse
		code, _, _ := request("gate.user.login", &protocol.TokenLoginRequest{
			AccessToken: refreshed[i].access, ServerId: 10001, DeviceId: "lt-scale",
		}, &rsp)
		codes[i] = code
	})
	fmt.Printf("revoked 40011=%d 其他=%d 墙钟=%s 返回码 %s\n", count(codes, 40011), n-count(codes, 40011), time.Since(start).Round(time.Millisecond), formatCounts(codes))
	return loginOK && refreshOK && replayOK && logoutOK && count(codes, 40011) == n
}

func lt03(n int) bool {
	fmt.Printf("== LT-03 准备 %d 条已登录连接 ==\n", n)
	clients := make([]*pomeloClient.Client, n)
	playerIDs := make([]int64, n)
	lines := make([]int32, n)
	codes := make([]int, n)
	stamp := time.Now().UnixNano()
	start := time.Now()
	parallel(n, func(i int) {
		c := pomeloClient.New(pomeloClient.WithRequestTimeout(20 * time.Second))
		if err := c.ConnectToWS(addr, ""); err != nil {
			codes[i] = -1
			return
		}
		clients[i] = c
		var issued protocol.IssueTokenResponse
		code, err := call(c, "gate.user.issueToken", &protocol.IssueTokenRequest{
			Nickname: fmt.Sprintf("c%d-%d", stamp, i),
			Password: "123456",
			DeviceId: "lt-scale",
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
			AccessToken: issued.AccessToken, ServerId: 10001, DeviceId: "lt-scale",
		}, &login)
		if err != nil || login.Uid < 1 {
			if code == 0 {
				code = -3
			}
			codes[i] = code
		}
	})
	printStats("prepare", time.Since(start), codes, nil)
	if count(codes, 0) != n {
		closeAll(clients)
		return false
	}

	fmt.Printf("== LT-03 同时选角 %d ==\n", n)
	codes = make([]int, n)
	lats := make([]time.Duration, n)
	start = time.Now()
	parallel(n, func(i int) {
		t := time.Now()
		var rsp protocol.PlayerSelectResponse
		code, err := call(clients[i], "game.player.select", &protocol.None{}, &rsp)
		lats[i] = time.Since(t)
		if err != nil || len(rsp.List) > 0 {
			if code == 0 {
				code = -4
			}
			codes[i] = code
		}
	})
	printStats("select", time.Since(start), codes, lats)
	selectOK := count(codes, 0) == n

	fmt.Printf("== LT-03 同时创角 %d ==\n", n)
	codes = make([]int, n)
	lats = make([]time.Duration, n)
	start = time.Now()
	parallel(n, func(i int) {
		t := time.Now()
		var rsp protocol.PlayerCreateResponse
		code, err := call(clients[i], "game.player.create", &protocol.PlayerCreateRequest{
			Name: fmt.Sprintf("p%d-%d", stamp%1000000, i),
		}, &rsp)
		lats[i] = time.Since(t)
		if err != nil || rsp.Player == nil || rsp.Player.PlayerId < 1 {
			if code == 0 {
				code = -3
			}
			codes[i] = code
			return
		}
		playerIDs[i] = rsp.Player.PlayerId
	})
	printStats("create", time.Since(start), codes, lats)
	createOK := count(codes, 0) == n

	fmt.Printf("== LT-03 同时进主城 %d ==\n", n)
	codes = make([]int, n)
	lats = make([]time.Duration, n)
	start = time.Now()
	parallel(n, func(i int) {
		if playerIDs[i] < 1 {
			codes[i] = -3
			return
		}
		t := time.Now()
		var rsp protocol.EnterGameResponse
		code, err := call(clients[i], "game.player.enter", &protocol.EnterGameRequest{
			PlayerId: playerIDs[i], SceneId: 1,
		}, &rsp)
		lats[i] = time.Since(t)
		if err != nil || rsp.SceneId != 1 || rsp.Line < 1 {
			if code == 0 {
				code = -5
			}
			codes[i] = code
			return
		}
		lines[i] = rsp.Line
	})
	printStats("enter", time.Since(start), codes, lats)
	fmt.Printf("主城分线 %s\n", formatLines(lines))
	closeAll(clients)
	return selectOK && createOK && count(codes, 0) == n
}

func parallel(n int, fn func(i int)) {
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			fn(i)
		}(i)
	}
	wg.Wait()
}

func loginAndLogout(p pair) (int, time.Duration) {
	c := pomeloClient.New(pomeloClient.WithRequestTimeout(20 * time.Second))
	if err := c.ConnectToWS(addr, ""); err != nil {
		return -1, 0
	}
	defer c.Disconnect()
	if _, err := c.Request("gate.user.login", &protocol.TokenLoginRequest{
		AccessToken: p.access, ServerId: 10001, DeviceId: "lt-scale",
	}); err != nil {
		return statusCode(err), 0
	}
	start := time.Now()
	if _, err := c.Request("gate.user.logout", &protocol.LogoutRequest{
		AccessToken: p.access, RefreshToken: p.refresh,
	}); err != nil {
		return statusCode(err), time.Since(start)
	}
	return 0, time.Since(start)
}

func request(route string, req, rsp interface{}) (int, time.Duration, error) {
	c := pomeloClient.New(pomeloClient.WithRequestTimeout(20 * time.Second))
	start := time.Now()
	if err := c.ConnectToWS(addr, ""); err != nil {
		return -1, time.Since(start), err
	}
	defer c.Disconnect()
	code, err := call(c, route, req, rsp)
	return code, time.Since(start), err
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

func closeAll(clients []*pomeloClient.Client) {
	for _, c := range clients {
		if c != nil {
			c.Disconnect()
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

func formatLines(lines []int32) string {
	counts := map[int32]int{}
	for _, line := range lines {
		if line > 0 {
			counts[line]++
		}
	}
	keys := make([]int, 0, len(counts))
	for k := range counts {
		keys = append(keys, int(k))
	}
	sort.Ints(keys)
	if len(keys) == 0 {
		return "无"
	}
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%d线×%d", k, counts[int32(k)]))
	}
	return strings.Join(parts, " ")
}
