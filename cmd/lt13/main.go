package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	pomeloClient "github.com/cherry-game/cherry/net/parser/pomelo/client"
	"github.com/example/mmo-server/internal/protocol"
)

const loadN = 50

var (
	addr    string
	gmAddr  string
	gmToken string
)

type slot struct {
	c        *pomeloClient.Client
	uid      int64
	nick     string
	pass     string
	refresh  string
	deviceID string
}

func main() {
	flag.StringVar(&addr, "addr", "gate-entry:10100", "gate entry host:port")
	flag.StringVar(&gmAddr, "gm", "gm:9080", "gm http host:port")
	flag.StringVar(&gmToken, "token", "dev-gm-token", "gm http token")
	flag.Parse()
	gm := newGM(gmAddr, gmToken)
	gm.maintenance(false, "")
	defer gm.maintenance(false, "")
	if err := waitReady(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if !fullChain(gm) || !load(gm) {
		os.Exit(1)
	}
}

func fullChain(gm *gmClient) bool {
	fmt.Println("== 全链路 2 人禁言、封号吊销、强制维护 ==")
	slots := make([]*slot, 2)
	if !enterMain(slots, "fc") {
		closeAll(slots)
		return false
	}
	defer func() {
		for _, s := range slots {
			if s != nil && s.uid > 0 {
				gm.unmute(s.uid)
				gm.unban(s.uid)
			}
		}
		gm.maintenance(false, "")
		closeAll(slots)
	}()
	ok := true
	a, b := slots[0], slots[1]
	code, lat := gm.mute(a.uid)
	fmt.Printf("禁言 code=%d 耗时=%s\n", code, lat.Round(time.Millisecond))
	ok = ok && code == 0
	ac, _ := chat(a, "muted")
	bc, _ := chat(b, "ok")
	fmt.Printf("禁言中聊天 甲=%d 乙=%d\n", ac, bc)
	ok = ok && ac == 40034 && bc == 0
	code, _ = gm.unmute(a.uid)
	ac, _ = chat(a, "back")
	fmt.Printf("解禁 code=%d 再聊天=%d\n", code, ac)
	ok = ok && code == 0 && ac == 0

	code, lat = gm.ban(a.uid)
	fmt.Printf("封号 code=%d 耗时=%s\n", code, lat.Round(time.Millisecond))
	ok = ok && code == 0
	rc, _ := refreshNew(a.refresh, a.deviceID)
	ic, _ := issueNew(a.nick, a.pass, a.deviceID)
	fmt.Printf("封号后刷新=%d 签发=%d\n", rc, ic)
	ok = ok && (rc == 40033 || rc == 40012) && ic == 40033
	code, _ = gm.unban(a.uid)
	ic, tokens := issueNew(a.nick, a.pass, a.deviceID)
	fmt.Printf("解封 code=%d 再签发=%d\n", code, ic)
	ok = ok && code == 0 && ic == 0 && tokens.access != ""

	code, lat = gm.maintenance(true, "lt13")
	fmt.Printf("开维护 code=%d 耗时=%s\n", code, lat.Round(time.Millisecond))
	ic, _ = issueNew(b.nick, b.pass, b.deviceID)
	fmt.Printf("维护中签发=%d\n", ic)
	ok = ok && code == 0 && ic == 40035
	code, _ = gm.maintenance(false, "")
	ic, _ = issueNew(b.nick, b.pass, b.deviceID)
	fmt.Printf("关维护 code=%d 再签发=%d\n", code, ic)
	ok = ok && code == 0 && ic == 0
	if ok {
		fmt.Println("全链路通过")
	}
	return ok
}

func load(gm *gmClient) bool {
	fmt.Printf("== 压测 %d 人并发禁言、封号、维护 ==\n", loadN)
	slots := make([]*slot, loadN)
	if !enterMain(slots, "ld") {
		closeAll(slots)
		return false
	}
	defer func() {
		for _, s := range slots {
			if s != nil && s.uid > 0 {
				gm.unmute(s.uid)
				gm.unban(s.uid)
			}
		}
		gm.maintenance(false, "")
		closeAll(slots)
	}()
	ok := true
	codes := make([]int, loadN)
	lats := make([]time.Duration, loadN)
	var wg sync.WaitGroup
	start := time.Now()
	for i := range slots {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c, d := gm.mute(slots[i].uid)
			codes[i], lats[i] = c, d
		}(i)
	}
	wg.Wait()
	printStats("mute", time.Since(start), codes, lats)
	ok = ok && count(codes, 0) == loadN

	start = time.Now()
	for i := range slots {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c, d := chat(slots[i], "muted")
			codes[i], lats[i] = c, d
		}(i)
	}
	wg.Wait()
	printStats("chat-muted", time.Since(start), codes, lats)
	ok = ok && count(codes, 40034) == loadN

	start = time.Now()
	for i := range slots {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c, d := gm.unmute(slots[i].uid)
			codes[i], lats[i] = c, d
		}(i)
	}
	wg.Wait()
	printStats("unmute", time.Since(start), codes, lats)
	ok = ok && count(codes, 0) == loadN

	start = time.Now()
	for i := range slots {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c, d := chat(slots[i], "back")
			codes[i], lats[i] = c, d
		}(i)
	}
	wg.Wait()
	printStats("chat-open", time.Since(start), codes, lats)
	ok = ok && count(codes, 0) == loadN

	start = time.Now()
	for i := range slots {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c, d := gm.ban(slots[i].uid)
			codes[i], lats[i] = c, d
		}(i)
	}
	wg.Wait()
	printStats("ban", time.Since(start), codes, lats)
	ok = ok && count(codes, 0) == loadN

	start = time.Now()
	for i := range slots {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c, d := refreshNew(slots[i].refresh, slots[i].deviceID)
			codes[i], lats[i] = c, d
		}(i)
	}
	wg.Wait()
	printStats("refresh-revoked", time.Since(start), codes, lats)
	revoked := count(codes, 40033) + count(codes, 40012)
	fmt.Printf("吊销后刷新 40033或40012=%d\n", revoked)
	ok = ok && revoked == loadN

	start = time.Now()
	for i := range slots {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c, d := issueOnly(slots[i].nick, slots[i].pass, slots[i].deviceID)
			codes[i], lats[i] = c, d
		}(i)
	}
	wg.Wait()
	printStats("issue-banned", time.Since(start), codes, lats)
	ok = ok && count(codes, 40033) == loadN

	start = time.Now()
	for i := range slots {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c, d := gm.unban(slots[i].uid)
			codes[i], lats[i] = c, d
		}(i)
	}
	wg.Wait()
	printStats("unban", time.Since(start), codes, lats)
	ok = ok && count(codes, 0) == loadN

	code, lat := gm.maintenance(true, "lt13-load")
	fmt.Printf("开维护 code=%d 耗时=%s\n", code, lat.Round(time.Millisecond))
	ok = ok && code == 0
	start = time.Now()
	for i := range slots {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c, d := issueOnly(slots[i].nick, slots[i].pass, slots[i].deviceID)
			codes[i], lats[i] = c, d
		}(i)
	}
	wg.Wait()
	printStats("issue-maint", time.Since(start), codes, lats)
	ok = ok && count(codes, 40035) == loadN
	code, _ = gm.maintenance(false, "")
	fmt.Printf("关维护 code=%d\n", code)
	ok = ok && code == 0
	start = time.Now()
	for i := range slots {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c, d := issueOnly(slots[i].nick, slots[i].pass, slots[i].deviceID)
			codes[i], lats[i] = c, d
		}(i)
	}
	wg.Wait()
	printStats("issue-after", time.Since(start), codes, lats)
	ok = ok && count(codes, 0) == loadN
	if ok {
		fmt.Println("压测通过")
	}
	return ok
}

func chat(s *slot, text string) (int, time.Duration) {
	if s == nil || s.c == nil {
		return -1, 0
	}
	t := time.Now()
	code, err := call(s.c, "game.chat.send", &protocol.ChatSendRequest{Text: text}, &protocol.None{})
	if err != nil && code == 0 {
		code = -2
	}
	return code, time.Since(t)
}

type issued struct {
	access  string
	refresh string
}

func issueNew(nick, pass, device string) (int, issued) {
	c := pomeloClient.New(pomeloClient.WithRequestTimeout(20 * time.Second))
	if err := c.ConnectToWS(addr, ""); err != nil {
		return -1, issued{}
	}
	defer c.Disconnect()
	return issueOn(c, nick, pass, device)
}

func issueOnly(nick, pass, device string) (int, time.Duration) {
	t := time.Now()
	code, _ := issueNew(nick, pass, device)
	return code, time.Since(t)
}

func issueOn(c *pomeloClient.Client, nick, pass, device string) (int, issued) {
	var rsp protocol.IssueTokenResponse
	code, err := call(c, "gate.user.issueToken", &protocol.IssueTokenRequest{
		Nickname: nick, Password: pass, DeviceId: device,
	}, &rsp)
	if err != nil && code == 0 {
		code = -2
	}
	return code, issued{access: rsp.AccessToken, refresh: rsp.RefreshToken}
}

func refreshNew(refresh, _ string) (int, time.Duration) {
	t := time.Now()
	c := pomeloClient.New(pomeloClient.WithRequestTimeout(20 * time.Second))
	if err := c.ConnectToWS(addr, ""); err != nil {
		return -1, time.Since(t)
	}
	defer c.Disconnect()
	code, err := call(c, "gate.user.refreshToken", &protocol.RefreshTokenRequest{
		RefreshToken: refresh,
	}, &protocol.RefreshTokenResponse{})
	if err != nil && code == 0 {
		code = -2
	}
	return code, time.Since(t)
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
			s := &slot{
				nick:     fmt.Sprintf("%s%d-%d", tag, stamp, i),
				pass:     "123456",
				deviceID: "lt13",
			}
			slots[i] = s
			c := pomeloClient.New(pomeloClient.WithRequestTimeout(20 * time.Second))
			if err := c.ConnectToWS(addr, ""); err != nil {
				codes[i] = -1
				return
			}
			s.c = c
			code, tok := issueOn(c, s.nick, s.pass, s.deviceID)
			if code != 0 || tok.access == "" {
				if code == 0 {
					code = -3
				}
				codes[i] = code
				return
			}
			s.refresh = tok.refresh
			var login protocol.TokenLoginResponse
			code, err := call(c, "gate.user.login", &protocol.TokenLoginRequest{
				AccessToken: tok.access, ServerId: 10001, DeviceId: s.deviceID,
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
				Name: fmt.Sprintf("m%d-%d", stamp%1000000, i),
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

type gmClient struct {
	base  string
	token string
	hc    *http.Client
}

func newGM(base, token string) *gmClient {
	return &gmClient{base: base, token: token, hc: &http.Client{Timeout: 20 * time.Second}}
}

func (g *gmClient) mute(uid int64) (int, time.Duration) {
	return g.post("/gm/account/mute", map[string]any{"uid": uid, "reason": "lt13", "durationSeconds": 0})
}

func (g *gmClient) unmute(uid int64) (int, time.Duration) {
	return g.post("/gm/account/unmute", map[string]any{"uid": uid})
}

func (g *gmClient) ban(uid int64) (int, time.Duration) {
	return g.post("/gm/account/ban", map[string]any{"uid": uid, "reason": "lt13", "durationSeconds": 0})
}

func (g *gmClient) unban(uid int64) (int, time.Duration) {
	return g.post("/gm/account/unban", map[string]any{"uid": uid})
}

func (g *gmClient) maintenance(on bool, reason string) (int, time.Duration) {
	return g.post("/gm/maintenance", map[string]any{"enabled": on, "reason": reason})
}

func (g *gmClient) post(path string, body any) (int, time.Duration) {
	raw, _ := json.Marshal(body)
	t := time.Now()
	req, err := http.NewRequest(http.MethodPost, "http://"+g.base+path, bytes.NewReader(raw))
	if err != nil {
		return -2, time.Since(t)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GM-Token", g.token)
	return g.do(req), time.Since(t)
}

func (g *gmClient) do(req *http.Request) int {
	resp, err := g.hc.Do(req)
	if err != nil {
		return -1
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var wrap struct {
		Code int32 `json:"code"`
	}
	if json.Unmarshal(b, &wrap) == nil && (wrap.Code != 0 || resp.StatusCode >= 400) {
		if wrap.Code != 0 {
			return int(wrap.Code)
		}
	}
	if resp.StatusCode >= 400 {
		return resp.StatusCode
	}
	return int(wrap.Code)
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
	if lats != nil {
		major, best := 0, -1
		seen := map[int]int{}
		for _, c := range codes {
			seen[c]++
			if seen[c] > best {
				best = seen[c]
				major = c
			}
		}
		for i, c := range codes {
			if c == major {
				timed = append(timed, lats[i])
			}
		}
		sort.Slice(timed, func(i, j int) bool { return timed[i] < timed[j] })
	}
	fmt.Printf("%s 墙钟=%s 返回码 %s\n", name, wall.Round(time.Millisecond), formatCounts(codes))
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
