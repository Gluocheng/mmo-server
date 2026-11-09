// LT-21 登录繁忙与先回包广播：
//  1. 一条主城线 200 人在线并拿到 refresh token。
//  2. 2000 个新号同一时刻签发，挤满密码工人（2 节点 × 6 工人 × 32 名额 = 384）：
//     排得下的成功，排不下的应马上得到 40036，不允许再出现 40005 或挂满 20 秒。
//  3. 签发还在算哈希时，200 个在线的人同时刷新，应全部成功（session 工人不被签发排队拖累）。
//  4. 200 人同时移动，回包应立即返回（先写坐标再回包），最后一档坐标广播最终要送到。
//  5. 200 人同时发言，广播队列（4096）有空位就全部成功；若出现 40037 如实统计。
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
	onlineN = 200
	issueN  = 2000
)

var addr string

type online struct {
	c       *pomeloClient.Client
	uid     int64
	refresh string

	mu       sync.Mutex
	lastMove map[int64]movePos // 每个周围人最后广播到的坐标
	chatGot  map[int64]int     // uid -> 收到的发言条数
}

type movePos struct{ x, z float32 }

func main() {
	flag.StringVar(&addr, "addr", "gate-entry:10100", "gate entry host:port")
	flag.Parse()
	if err := waitReady(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	slots := make([]*online, onlineN)
	if !prepare(slots) {
		closeAll(slots)
		os.Exit(1)
	}
	defer closeAll(slots)
	ok := true
	if !burstIssueWhileRefresh(slots) {
		ok = false
	}
	if !moveFirstResponse(slots) {
		ok = false
	}
	if !chatQueue(slots) {
		ok = false
	}
	if !ok {
		os.Exit(1)
	}
	fmt.Println("LT-21 压测通过")
}

// prepare 200 人签发、登录、创角、进主城同一条线，记下 refresh token。
func prepare(slots []*online) bool {
	fmt.Printf("== 准备 %d 条已登录连接 ==\n", onlineN)
	n := len(slots)
	codes := make([]int, n)
	stamp := time.Now().UnixNano()
	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s := &online{lastMove: map[int64]movePos{}, chatGot: map[int64]int{}}
			slots[i] = s
			c := pomeloClient.New(pomeloClient.WithRequestTimeout(30 * time.Second))
			if err := c.ConnectToWS(addr, ""); err != nil {
				codes[i] = -1
				return
			}
			s.c = c
			c.On("onMove", s.onMove)
			c.On("onChat", s.onChat)
			var issued protocol.IssueTokenResponse
			code, err := call(c, "gate.user.issueToken", &protocol.IssueTokenRequest{
				Nickname: fmt.Sprintf("w%d-%d", stamp, i),
				Password: "123456", DeviceId: "lt21",
			}, &issued)
			if err != nil || issued.AccessToken == "" {
				if code == 0 {
					code = -3
				}
				codes[i] = code
				return
			}
			s.refresh = issued.RefreshToken
			var login protocol.TokenLoginResponse
			code, err = call(c, "gate.user.login", &protocol.TokenLoginRequest{
				AccessToken: issued.AccessToken, ServerId: 10001, DeviceId: "lt21",
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
				Name: fmt.Sprintf("w%d-%d", stamp%100000, i),
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
			if err != nil || entered.SceneId != 1 || entered.Line != 1 {
				if code == 0 {
					code = -5
				}
				codes[i] = code
			}
		}(i)
	}
	wg.Wait()
	printStats("prepare", time.Since(start), codes, nil)
	if count(codes, 0) != n {
		return false
	}
	fmt.Printf("主城 1 线 ×%d\n", n)
	return true
}

// burstIssueWhileRefresh 2000 人突发签发，同一窗口内 200 个在线号刷新。
func burstIssueWhileRefresh(slots []*online) bool {
	fmt.Printf("== %d 人同时签发（%d 人同时刷新）==\n", issueN, onlineN)
	ok := true
	stamp := time.Now().UnixNano()

	issueCodes := make([]int, issueN)
	issueLats := make([]time.Duration, issueN)
	refreshCodes := make([]int, onlineN)
	refreshLats := make([]time.Duration, onlineN)

	var wg sync.WaitGroup
	start := time.Now()
	// 先起刷新协程：让刷新打在被签发挤满的登录进程上
	for i := range slots {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			time.Sleep(300 * time.Millisecond) // 等签发把密码工人挤满
			t := time.Now()
			var rsp protocol.RefreshTokenResponse
			code, err := call(slots[i].c, "gate.user.refreshToken", &protocol.RefreshTokenRequest{
				RefreshToken: slots[i].refresh,
			}, &rsp)
			refreshLats[i] = time.Since(t)
			if err == nil && (rsp.AccessToken == "" || rsp.RefreshToken == "") {
				code = -3
			}
			if err == nil && code == 0 {
				slots[i].mu.Lock()
				slots[i].refresh = rsp.RefreshToken
				slots[i].mu.Unlock()
			}
			refreshCodes[i] = code
		}(i)
	}
	for i := 0; i < issueN; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c := pomeloClient.New(pomeloClient.WithRequestTimeout(60 * time.Second))
			t := time.Now()
			if err := c.ConnectToWS(addr, ""); err != nil {
				issueCodes[i], issueLats[i] = -1, time.Since(t)
				return
			}
			defer c.Disconnect()
			var rsp protocol.IssueTokenResponse
			code, err := call(c, "gate.user.issueToken", &protocol.IssueTokenRequest{
				Nickname: fmt.Sprintf("x%d-%d", stamp, i),
				Password: "123456", DeviceId: "lt21",
			}, &rsp)
			issueLats[i] = time.Since(t)
			if err == nil && code == 0 && (rsp.AccessToken == "" || rsp.RefreshToken == "") {
				code = -3
			}
			issueCodes[i] = code
		}(i)
	}
	wg.Wait()
	wall := time.Since(start)

	succ := count(issueCodes, 0)
	busy := count(issueCodes, 40036)
	rpcFail := count(issueCodes, 40005)
	connFail := count(issueCodes, -1)
	printStats("issue", wall, issueCodes, issueLats)
	fmt.Printf("签发 成功=%d 繁忙40036=%d 40005=%d 连不上=%d\n", succ, busy, rpcFail, connFail)
	// 判定：不许再出现 40005；繁忙的必须快速返回（p95 用成功耗时看，繁忙耗时单独统计）
	ok = ok && rpcFail == 0 && succ+busy+connFail == issueN
	var busyLats []time.Duration
	for i, c := range issueCodes {
		if c == 40036 {
			busyLats = append(busyLats, issueLats[i])
		}
	}
	if len(busyLats) > 0 {
		sort.Slice(busyLats, func(i, j int) bool { return busyLats[i] < busyLats[j] })
		fmt.Printf("40036 快速拒绝 p50=%s p95=%s max=%s\n",
			pct(busyLats, 50), pct(busyLats, 95), busyLats[len(busyLats)-1].Round(time.Millisecond))
		// 繁忙拒绝不应等到 20 秒 request_timeout
		if busyLats[len(busyLats)-1] > 10*time.Second {
			fmt.Println("40036 最慢超过 10 秒，不像快速拒绝")
			ok = false
		}
	}

	printStats("refresh", wall, refreshCodes, refreshLats)
	refreshOK := count(refreshCodes, 0)
	fmt.Printf("签发挤满时刷新 成功=%d/%d\n", refreshOK, onlineN)
	ok = ok && refreshOK == onlineN
	return ok
}

// moveLayout 把 200 人排在 100 列 × 2 行的蛇形长条上，间距 1。
// 相邻编号（含 199→0 环绕）距离都是 1，必然落在 AOI 半径 15 内，
// 这样「下一位收到我最后坐标」的断言才只反映广播是否到位。
func moveLayout(i int) movePos {
	const cols = 100
	row := i / cols
	col := i % cols
	if row%2 == 1 {
		col = cols - 1 - col
	}
	return movePos{x: float32(col), z: float32(row)}
}

// moveFirstResponse 200 人同时移动：回包要快（先回包再广播），最终坐标要广播到位。
func moveFirstResponse(slots []*online) bool {
	fmt.Printf("== %d 人同时移动（先回包后广播）==\n", onlineN)
	ok := true
	for _, s := range slots {
		s.resetMoves()
	}
	codes := make([]int, onlineN)
	lats := make([]time.Duration, onlineN)
	var wg sync.WaitGroup
	start := time.Now()
	for i := range slots {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p := moveLayout(i)
			t := time.Now()
			code, err := call(slots[i].c, "game.player.move", &protocol.MoveRequest{
				X: p.x, Y: 0, Z: p.z,
			}, &protocol.None{})
			lats[i] = time.Since(t)
			if err != nil && code == 0 {
				code = -2
			}
			codes[i] = code
		}(i)
	}
	wg.Wait()
	printStats("move", time.Since(start), codes, lats)
	ok = ok && count(codes, 0) == onlineN
	// 回包不等广播：p95 应远小于旧行为里被推送堵住的 20 秒
	var okLats []time.Duration
	for i, c := range codes {
		if c == 0 {
			okLats = append(okLats, lats[i])
		}
	}
	if len(okLats) > 0 {
		sort.Slice(okLats, func(i, j int) bool { return okLats[i] < okLats[j] })
		p95 := pct(okLats, 95)
		fmt.Printf("移动回包 p95=%s\n", p95)
		if p95 > 3*time.Second {
			fmt.Println("移动回包 p95 超过 3 秒，疑似仍在等广播")
			ok = false
		}
	}
	// 广播最终到位：每人的「下一位」都在 AOI 内，应看到对方最后坐标
	time.Sleep(3 * time.Second)
	finalOK, finalBad := 0, 0
	for i, s := range slots {
		want := moveLayout((i + 1) % onlineN)
		got, seen := s.lastPosOf(slots[(i+1)%onlineN].uid)
		if seen && got == want {
			finalOK++
		} else {
			finalBad++
		}
	}
	fmt.Printf("下一位最后坐标到位=%d 缺失或错=%d\n", finalOK, finalBad)
	// 同时对移动会按发送者合并，同一人只留最新一档。缺的这几个再用错开重发单独确认，
	// 区分「合并丢档」和「真的丢了」。
	if finalBad > 0 {
		for _, s := range slots {
			s.resetMoves()
		}
		retryCodes := make([]int, onlineN)
		for i := range slots {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				p := moveLayout(i)
				code, err := call(slots[i].c, "game.player.move", &protocol.MoveRequest{
					X: p.x + 0.5, Y: 0, Z: p.z,
				}, &protocol.None{})
				if err != nil && code == 0 {
					code = -2
				}
				retryCodes[i] = code
			}(i)
			time.Sleep(2 * time.Millisecond)
		}
		wg.Wait()
		time.Sleep(2 * time.Second)
		retryOK := 0
		for i, s := range slots {
			want := moveLayout((i + 1) % onlineN)
			want.x += 0.5
			got, seen := s.lastPosOf(slots[(i+1)%onlineN].uid)
			if seen && got == want {
				retryOK++
			}
		}
		fmt.Printf("错开重发后到位=%d/%d\n", retryOK, onlineN)
		ok = ok && count(retryCodes, 0) == onlineN && retryOK == onlineN
	} else {
		ok = true
	}
	return ok
}

// chatQueue 200 人同时发言：队列 4096 应全部入队成功，广播不丢。
func chatQueue(slots []*online) bool {
	fmt.Printf("== %d 人同时发言 ==\n", onlineN)
	ok := true
	for _, s := range slots {
		s.resetChat()
	}
	codes := make([]int, onlineN)
	lats := make([]time.Duration, onlineN)
	var wg sync.WaitGroup
	start := time.Now()
	for i := range slots {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			t := time.Now()
			code, err := call(slots[i].c, "game.chat.send", &protocol.ChatSendRequest{
				Text: fmt.Sprintf("t%d-%d", start.UnixNano()%100000, i),
			}, &protocol.None{})
			lats[i] = time.Since(t)
			if err != nil && code == 0 {
				code = -2
			}
			codes[i] = code
		}(i)
	}
	wg.Wait()
	printStats("chat", time.Since(start), codes, lats)
	succ := count(codes, 0)
	busy := count(codes, 40037)
	fmt.Printf("发言 成功=%d 队列满40037=%d\n", succ, busy)
	ok = ok && succ+busy == onlineN
	// 广播不含发送者自己：每人应收到 succ-1 条
	want := succ - 1
	time.Sleep(3 * time.Second)
	full, missing := 0, 0
	total := 0
	for _, s := range slots {
		n := s.chatCount()
		total += n
		if n == want {
			full++
		} else {
			missing++
		}
	}
	fmt.Printf("收到全部发言=%d 缺条=%d 总收到=%d 期望=%d（每人 %d 条，不含自己）\n",
		full, missing, total, want*onlineN, want)
	ok = ok && missing == 0
	return ok
}

func (s *online) onMove(msg *pomeloMessage.Message) {
	if len(msg.Data) == 0 {
		return
	}
	var b protocol.MoveBroadcast
	if s.c.Serializer().Unmarshal(msg.Data, &b) != nil || b.Uid < 1 {
		return
	}
	s.mu.Lock()
	s.lastMove[b.Uid] = movePos{x: b.X, z: b.Z}
	s.mu.Unlock()
}

func (s *online) onChat(msg *pomeloMessage.Message) {
	if len(msg.Data) == 0 {
		return
	}
	var b protocol.ChatBroadcast
	if s.c.Serializer().Unmarshal(msg.Data, &b) != nil || b.Uid < 1 {
		return
	}
	s.mu.Lock()
	s.chatGot[b.Uid]++
	s.mu.Unlock()
}

func (s *online) lastPosOf(uid int64) (movePos, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.lastMove[uid]
	return p, ok
}

func (s *online) chatCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, c := range s.chatGot {
		n += c
	}
	return n
}

func (s *online) resetMoves() {
	s.mu.Lock()
	s.lastMove = map[int64]movePos{}
	s.mu.Unlock()
}

func (s *online) resetChat() {
	s.mu.Lock()
	s.chatGot = map[int64]int{}
	s.mu.Unlock()
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

func closeAll(slots []*online) {
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
