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
	pomeloMessage "github.com/cherry-game/cherry/net/parser/pomelo/message"
	"github.com/example/mmo-server/internal/protocol"
)

const (
	loadN      = 50         // 压测人数
	mainScene  = int32(1)   // 主城
	wildScene  = int32(3)   // 荒野，用来验证公告按场景隔离
	emptyScene = int32(999) // 无人场景，用来验证公告推送人数为 0
)

var (
	addr    string
	gmAddr  string
	gmToken string
)

// slot 是一名压测客户端：一条网关连接 + 收到的公告。
type slot struct {
	c   *pomeloClient.Client
	uid int64
	mu  sync.Mutex
	got map[string]int // 公告文本 -> 收到次数
	n   int            // 收到的公告总条数
}

func main() {
	flag.StringVar(&addr, "addr", "gate-entry:10100", "gate entry host:port")
	flag.StringVar(&gmAddr, "gm", "gm:9080", "gm http host:port")
	flag.StringVar(&gmToken, "token", "dev-gm-token", "gm http token")
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
	fmt.Println("== 全链路 2 人 GM 公告与场景在线 ==")
	slots := make([]*slot, 2)
	if !enterMain(slots, "fc") {
		closeAll(slots)
		return false
	}
	gm := newGM(gmAddr, gmToken)
	ok := true

	// 在线列表：场景 1 应包含这两个 uid
	list, code, lat := gm.online(mainScene)
	fmt.Printf("online scene=%d code=%d 耗时=%s 人数=%d\n", mainScene, code, lat.Round(time.Millisecond), len(list))
	ok = ok && code == 0 && hasAll(list, slots) && linesOK(list)

	// 场景公告：两人都应收到同一条
	text := fmt.Sprintf("全链路公告-%d", time.Now().UnixNano())
	pushed, code, lat := gm.notice(mainScene, text)
	fmt.Printf("notice scene=%d code=%d 耗时=%s 推送人数=%d\n", mainScene, code, lat.Round(time.Millisecond), pushed)
	ok = ok && code == 0 && pushed >= 2
	waitNotice(slots, text, 2)
	for _, s := range slots {
		n := s.hits(text)
		fmt.Printf("uid=%d 收到公告=%d 次\n", s.uid, n)
		ok = ok && n == 1
	}

	// 全服公告：scene_id=0 同样应到达
	textAll := fmt.Sprintf("全服公告-%d", time.Now().UnixNano())
	pushed, code, _ = gm.notice(0, textAll)
	fmt.Printf("notice 全服 code=%d 推送人数=%d\n", code, pushed)
	ok = ok && code == 0 && pushed >= 2
	waitNotice(slots, textAll, 2)
	for _, s := range slots {
		n := s.hits(textAll)
		fmt.Printf("uid=%d 收到全服公告=%d 次\n", s.uid, n)
		ok = ok && n == 1
	}

	// 空场景公告：不应推给任何人
	pushed, code, _ = gm.notice(emptyScene, fmt.Sprintf("空场景-%d", time.Now().UnixNano()))
	fmt.Printf("notice scene=%d code=%d 推送人数=%d\n", emptyScene, code, pushed)
	ok = ok && code == 0 && pushed == 0

	// 跨场景隔离：一人切到荒野后，主城公告只应推到留下的那个人
	if sc, lat := switchScene(slots[0], wildScene); sc != 0 {
		fmt.Printf("uid=%d 切图到 %d 失败 code=%d\n", slots[0].uid, wildScene, sc)
		ok = false
	} else {
		fmt.Printf("uid=%d 切图到 %d 耗时=%s\n", slots[0].uid, wildScene, lat.Round(time.Millisecond))
	}
	textIso := fmt.Sprintf("隔离公告-%d", time.Now().UnixNano())
	pushed, code, _ = gm.notice(mainScene, textIso)
	fmt.Printf("notice scene=%d（一人已切图）code=%d 推送人数=%d\n", mainScene, code, pushed)
	ok = ok && code == 0 && pushed == 1
	time.Sleep(500 * time.Millisecond)
	moved, stayed := slots[0].hits(textIso), slots[1].hits(textIso)
	fmt.Printf("切图者收到=%d 留守者收到=%d\n", moved, stayed)
	ok = ok && moved == 0 && stayed == 1

	// 空文本公告：应拒绝，返回 40030
	_, bcode, _ := gm.notice(mainScene, "   ")
	fmt.Printf("notice 空文本 code=%d\n", bcode)
	ok = ok && bcode == 40030

	closeAll(slots)
	time.Sleep(500 * time.Millisecond)
	if ok {
		fmt.Println("全链路通过")
	}
	return ok
}

func load() bool {
	fmt.Printf("== 压测 %d 人 GM 公告与在线 ==", loadN)
	fmt.Println()
	slots := make([]*slot, loadN)
	if !enterMain(slots, "ld") {
		closeAll(slots)
		return false
	}
	gm := newGM(gmAddr, gmToken)
	ok := true

	// 阶段1：在线列表应一次性导出全部 50 人
	list, code, lat := gm.online(mainScene)
	fmt.Printf("online scene=%d code=%d 耗时=%s 人数=%d\n", mainScene, code, lat.Round(time.Millisecond), len(list))
	ok = ok && code == 0 && hasAll(list, slots) && linesOK(list)

	// 阶段2：50 条公告并发下发，每条都应推给全部 50 人
	stamp := time.Now().UnixNano()
	texts := make([]string, loadN)
	for i := range texts {
		texts[i] = fmt.Sprintf("ld-%d-%d", stamp, i)
	}
	codes := make([]int, loadN)
	pushN := make([]int, loadN)
	lats := make([]time.Duration, loadN)
	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < loadN; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			pushed, c, d := gm.notice(mainScene, texts[i])
			codes[i], pushN[i], lats[i] = c, int(pushed), d
		}(i)
	}
	wg.Wait()
	printStats("notice×50", time.Since(start), codes, lats)
	badPush := 0
	for i, n := range pushN {
		if n < loadN {
			badPush++
			if badPush <= 3 {
				fmt.Printf("第 %d 条公告只推到 %d 人\n", i, n)
			}
		}
	}
	fmt.Printf("notice 推送人数达标=%d 不足=%d\n", loadN-badPush, badPush)
	ok = ok && badPush == 0

	// 每条公告都必须完整到达每个客户端
	waitAll(gm, slots, texts, 5*time.Second)
	missing := 0
	for _, s := range slots {
		for _, t := range texts {
			if s.hits(t) != 1 {
				missing++
			}
		}
	}
	fmt.Printf("公告到达 应到=%d 缺失=%d\n", loadN*loadN, missing)
	ok = ok && missing == 0

	// 阶段3：50 次在线查询并发，每次都要拿到全部 50 人
	codes = make([]int, loadN)
	lats = make([]time.Duration, loadN)
	sizes := make([]int, loadN)
	start = time.Now()
	for i := 0; i < loadN; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			l, c, d := gm.online(mainScene)
			codes[i], lats[i] = c, d
			if c == 0 {
				sizes[i] = len(l)
			}
		}(i)
	}
	wg.Wait()
	printStats("online×50", time.Since(start), codes, lats)
	badSize := 0
	for _, n := range sizes {
		if n < loadN {
			badSize++
		}
	}
	fmt.Printf("online 人数达标=%d 偏少=%d\n", loadN-badSize, badSize)
	ok = ok && badSize == 0

	closeAll(slots)
	if ok {
		fmt.Println("压测通过")
	}
	return ok
}

// ---- GM HTTP ----

type gmClient struct {
	base  string
	token string
	hc    *http.Client
}

// onlineRow protojson 把 int64 输出成字符串，这里按字符串收再转回数字。
type onlineRow struct {
	Uid     string `json:"uid"`
	SceneId int32  `json:"sceneId"`
	Line    int32  `json:"line"`
}

func (r onlineRow) uid() int64 {
	n, _ := strconv.ParseInt(r.Uid, 10, 64)
	return n
}

type onlineRsp struct {
	Code int32       `json:"code"`
	List []onlineRow `json:"list"`
}

type noticeRsp struct {
	Code   int32 `json:"code"`
	Pushed int32 `json:"pushed"`
}

func newGM(base, token string) *gmClient {
	return &gmClient{base: base, token: token, hc: &http.Client{Timeout: 10 * time.Second}}
}

// notice 发公告，返回推送人数、业务码与耗时。
func (g *gmClient) notice(sceneID int32, text string) (int32, int, time.Duration) {
	body, _ := json.Marshal(map[string]any{"sceneId": sceneID, "text": text})
	t := time.Now()
	req, err := http.NewRequest(http.MethodPost, "http://"+g.base+"/gm/notice", bytes.NewReader(body))
	if err != nil {
		return 0, -2, time.Since(t)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GM-Token", g.token)
	var rsp noticeRsp
	code := g.do(req, &rsp)
	return rsp.Pushed, code, time.Since(t)
}

// online 查场景在线，返回列表、业务码与耗时。
func (g *gmClient) online(sceneID int32) ([]onlineRow, int, time.Duration) {
	t := time.Now()
	url := fmt.Sprintf("http://%s/gm/world/online?sceneId=%d", g.base, sceneID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, -2, time.Since(t)
	}
	req.Header.Set("X-GM-Token", g.token)
	var rsp onlineRsp
	code := g.do(req, &rsp)
	return rsp.List, code, time.Since(t)
}

// do 发请求并解出业务码；非 2xx 也读 body 拿业务码。
func (g *gmClient) do(req *http.Request, out any) int {
	rsp, err := g.hc.Do(req)
	if err != nil {
		return -1
	}
	defer rsp.Body.Close()
	data, _ := io.ReadAll(rsp.Body)
	if err := json.Unmarshal(data, out); err != nil {
		return -2
	}
	var head struct {
		Code int32 `json:"code"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return -2
	}
	return int(head.Code)
}

// hasAll 校验 slots 里每个 uid 都在在线列表内。
func hasAll(list []onlineRow, slots []*slot) bool {
	seen := make(map[int64]bool, len(list))
	for _, r := range list {
		seen[r.uid()] = true
	}
	for _, s := range slots {
		if !seen[s.uid] {
			return false
		}
	}
	return true
}

// linesOK 校验在线列表里都落在主城且线号有效。
func linesOK(list []onlineRow) bool {
	for _, r := range list {
		if r.SceneId != mainScene || r.Line < 1 {
			return false
		}
	}
	return true
}

// ---- 客户端 ----

func (s *slot) add(text string) {
	s.mu.Lock()
	if s.got == nil {
		s.got = make(map[string]int)
	}
	s.got[text]++
	s.n++
	s.mu.Unlock()
}

func (s *slot) hits(text string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.got[text]
}

// waitNotice 等每个客户端都收到该文本，最多等 2 秒。
func waitNotice(slots []*slot, text string, want int) {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		all := true
		for _, s := range slots {
			if s.hits(text) < want {
				all = false
				break
			}
		}
		if all {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// waitAll 等每个客户端把全部文本都收齐，或到时。
func waitAll(_ *gmClient, slots []*slot, texts []string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		all := true
		for _, s := range slots {
			for _, t := range texts {
				if s.hits(t) == 0 {
					all = false
					break
				}
			}
			if !all {
				break
			}
		}
		if all {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// enterMain 并发签发、登录、创角、进主城，返回是否全部成功。
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
			s := &slot{got: make(map[string]int)}
			slots[i] = s
			c := pomeloClient.New(pomeloClient.WithRequestTimeout(20 * time.Second))
			if err := c.ConnectToWS(addr, ""); err != nil {
				codes[i] = -1
				return
			}
			s.c = c
			c.On("onNotice", func(msg *pomeloMessage.Message) {
				var push protocol.GmNoticePush
				if len(msg.Data) == 0 || c.Serializer().Unmarshal(msg.Data, &push) != nil {
					return
				}
				s.add(push.Text)
			})
			var issued protocol.IssueTokenResponse
			code, err := call(c, "gate.user.issueToken", &protocol.IssueTokenRequest{
				Nickname: fmt.Sprintf("%s%d-%d", tag, stamp, i),
				Password: "123456",
				DeviceId: "lt12",
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
				AccessToken: issued.AccessToken, ServerId: 10001, DeviceId: "lt12",
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
				Name: fmt.Sprintf("g%d-%d", stamp%1000000, i),
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
			if err != nil || entered.SceneId != mainScene {
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

// switchScene 换图，返回业务码与耗时。
func switchScene(s *slot, sceneID int32) (int, time.Duration) {
	var rsp protocol.SceneSwitchResponse
	t := time.Now()
	code, err := call(s.c, "game.player.switchScene", &protocol.SceneSwitchRequest{SceneId: sceneID}, &rsp)
	if err != nil && code == 0 {
		code = -2
	}
	return code, time.Since(t)
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
