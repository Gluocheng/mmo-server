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
	loadN   = 200
	itemID  = int32(1001)
	bagType = int32(2)
)

var (
	addr    string
	gmAddr  string
	gmToken string
)

type slot struct {
	c         *pomeloClient.Client
	uid       int64
	playerID  int64
	mu        sync.Mutex
	pushes    int
	lastCount int32
	lastBytes int
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
	gm := newGM(gmAddr, gmToken)
	if !fullChain(gm) || !load(gm) {
		os.Exit(1)
	}
}

func fullChain(gm *gmClient) bool {
	fmt.Println("== 全链路 2 人发奖、扣道具、在线热更 ==")
	slots := make([]*slot, 2)
	if !enterMain(slots, "fc") {
		closeAll(slots)
		return false
	}
	defer closeAll(slots)
	ok := true
	a := slots[0]
	grant, lat := gm.grant(a.playerID, itemID, 10, bagType)
	fmt.Printf("发奖 code=%d pushed=%v 数量=%d 耗时=%s\n", grant.code, grant.pushed, grant.count, lat.Round(time.Millisecond))
	ok = ok && grant.code == 0 && grant.pushed && grant.count == 10
	ok = ok && a.waitPush(1, 2*time.Second)
	_, pushedCount := a.snapshot()
	fmt.Printf("甲推送数量=%d\n", pushedCount)
	ok = ok && pushedCount == 10

	bad := gm.mustGrant(a.playerID, 99999, 1, bagType)
	fmt.Printf("非法道具 code=%d\n", bad.code)
	ok = ok && bad.code == 40023
	wrong := gm.mustGrant(a.playerID, itemID, 1, 1)
	fmt.Printf("错误背包 code=%d\n", wrong.code)
	ok = ok && wrong.code == 40028

	deduct, lat := gm.deductItem(a.playerID, itemID, 4, bagType)
	fmt.Printf("按道具扣 code=%d 数量=%d 耗时=%s\n", deduct.code, deduct.count, lat.Round(time.Millisecond))
	ok = ok && deduct.code == 0 && deduct.count == 6
	slotDeduct, _ := gm.deductSlot(a.playerID, 0, 1, bagType)
	fmt.Printf("按格子扣 code=%d 数量=%d\n", slotDeduct.code, slotDeduct.count)
	ok = ok && slotDeduct.code == 0 && slotDeduct.count == 5
	ok = ok && a.waitPush(3, 2*time.Second)
	_, pushedCount = a.snapshot()
	fmt.Printf("扣完后推送数量=%d\n", pushedCount)
	ok = ok && pushedCount == 5
	lack := gm.mustDeductItem(a.playerID, itemID, 100, bagType)
	fmt.Printf("扣不够 code=%d\n", lack.code)
	ok = ok && lack.code == 40019

	reload, lat := gm.reload("bag_type")
	fmt.Printf("热更 code=%d version=%s tables=%d 耗时=%s\n", reload.code, reload.version, reload.tables, lat.Round(time.Millisecond))
	ok = ok && reload.code == 0 && reload.version != ""
	for i, s := range slots {
		c := playerSelect(s)
		fmt.Printf("热更后选角 %d code=%d\n", i, c)
		ok = ok && c == 0
	}
	if ok {
		fmt.Println("全链路通过")
	}
	return ok
}

func load(gm *gmClient) bool {
	fmt.Printf("== 压测 %d 人并发发奖、扣道具，并在在线时热更 ==\n", loadN)
	slots := make([]*slot, loadN)
	if !enterMain(slots, "ld") {
		closeAll(slots)
		return false
	}
	defer closeAll(slots)
	ok := true
	codes := make([]int, loadN)
	lats := make([]time.Duration, loadN)
	counts := make([]int32, loadN)
	gotPush := make([]bool, loadN)
	var wg sync.WaitGroup
	start := time.Now()
	for i := range slots {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rsp, d := gm.grant(slots[i].playerID, itemID, 10, bagType)
			codes[i], lats[i], counts[i] = rsp.code, d, rsp.count
			gotPush[i] = rsp.pushed
		}(i)
	}
	wg.Wait()
	printStats("grant", time.Since(start), codes, lats)
	fmt.Printf("发奖 pushed=%d 数量对=%d\n", countBool(gotPush), countN(counts, 10))
	ok = ok && count(codes, 0) == loadN && countBool(gotPush) == loadN && countN(counts, 10) == loadN
	time.Sleep(300 * time.Millisecond)
	pushOK := 0
	for _, s := range slots {
		_, n := s.snapshot()
		if n == 10 {
			pushOK++
		}
	}
	fmt.Printf("发奖推送数量为10=%d\n", pushOK)
	ok = ok && pushOK == loadN

	start = time.Now()
	for i := range slots {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rsp, d := gm.deductItem(slots[i].playerID, itemID, 4, bagType)
			codes[i], lats[i], counts[i] = rsp.code, d, rsp.count
		}(i)
	}
	wg.Wait()
	printStats("deduct", time.Since(start), codes, lats)
	fmt.Printf("扣后数量对=%d\n", countN(counts, 6))
	ok = ok && count(codes, 0) == loadN && countN(counts, 6) == loadN

	start = time.Now()
	for i := range slots {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rsp, d := gm.deductItem(slots[i].playerID, itemID, 100, bagType)
			codes[i], lats[i] = rsp.code, d
		}(i)
	}
	wg.Wait()
	printStats("deduct-short", time.Since(start), codes, lats)
	ok = ok && count(codes, 40019) == loadN

	reload, lat := gm.reload("bag_type")
	fmt.Printf("在线热更 code=%d version=%s tables=%d 耗时=%s\n", reload.code, reload.version, reload.tables, lat.Round(time.Millisecond))
	ok = ok && reload.code == 0 && reload.version != ""

	start = time.Now()
	for i := range slots {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			t := time.Now()
			codes[i], lats[i] = playerSelect(slots[i]), time.Since(t)
		}(i)
	}
	wg.Wait()
	printStats("select-after-reload", time.Since(start), codes, lats)
	ok = ok && count(codes, 0) == loadN
	if ok {
		fmt.Println("压测通过")
	}
	return ok
}

func playerSelect(s *slot) int {
	if s == nil || s.c == nil {
		return -1
	}
	code, err := call(s.c, "game.player.select", &protocol.None{}, &protocol.PlayerSelectResponse{})
	if err != nil && code == 0 {
		code = -2
	}
	return code
}

func (s *slot) waitPush(n int, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		pushes, _ := s.snapshot()
		if pushes >= n {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

func (s *slot) snapshot() (int, int32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pushes, s.lastCount
}

func (s *slot) onBag(msg *pomeloMessage.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pushes++
	s.lastBytes = len(msg.Data)
	if s.c == nil || len(msg.Data) == 0 {
		return
	}
	var bag protocol.BagListResponse
	if s.c.Serializer().Unmarshal(msg.Data, &bag) != nil {
		return
	}
	var n int32
	for _, it := range bag.Items {
		if it != nil && it.ItemId == itemID {
			n += it.Count
		}
	}
	s.lastCount = n
}

type bagOp struct {
	code   int
	pushed bool
	count  int32
}

type reloadRsp struct {
	code    int
	version string
	tables  int64
}

type gmClient struct {
	base  string
	token string
	hc    *http.Client
}

func newGM(base, token string) *gmClient {
	return &gmClient{base: base, token: token, hc: &http.Client{Timeout: 20 * time.Second}}
}

func (g *gmClient) grant(playerID int64, item, count, bag int32) (bagOp, time.Duration) {
	return g.op("/gm/bag/grant", map[string]any{
		"playerId": playerID, "itemId": item, "count": count, "bagType": bag,
	})
}

func (g *gmClient) mustGrant(playerID int64, item, count, bag int32) bagOp {
	op, _ := g.grant(playerID, item, count, bag)
	return op
}

func (g *gmClient) deductItem(playerID int64, item, count, bag int32) (bagOp, time.Duration) {
	return g.op("/gm/bag/deduct", map[string]any{
		"playerId": playerID, "itemId": item, "count": count, "bagType": bag,
	})
}

func (g *gmClient) mustDeductItem(playerID int64, item, count, bag int32) bagOp {
	op, _ := g.deductItem(playerID, item, count, bag)
	return op
}

func (g *gmClient) deductSlot(playerID int64, slot, count, bag int32) (bagOp, time.Duration) {
	return g.op("/gm/bag/deduct", map[string]any{
		"playerId": playerID, "slot": slot, "count": count, "bagType": bag,
	})
}

func (g *gmClient) op(path string, body any) (bagOp, time.Duration) {
	raw, _ := json.Marshal(body)
	t := time.Now()
	req, err := http.NewRequest(http.MethodPost, "http://"+g.base+path, bytes.NewReader(raw))
	if err != nil {
		return bagOp{code: -2}, time.Since(t)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GM-Token", g.token)
	resp, err := g.hc.Do(req)
	if err != nil {
		return bagOp{code: -1}, time.Since(t)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var wrap struct {
		Code   int32 `json:"code"`
		Pushed bool  `json:"pushed"`
		Bag    struct {
			Items []struct {
				ItemID int32 `json:"itemId"`
				Count  int32 `json:"count"`
			} `json:"items"`
		} `json:"bag"`
	}
	_ = json.Unmarshal(b, &wrap)
	if wrap.Code == 0 && resp.StatusCode >= 400 {
		return bagOp{code: resp.StatusCode}, time.Since(t)
	}
	var n int32
	for _, it := range wrap.Bag.Items {
		if it.ItemID == itemID {
			n += it.Count
		}
	}
	return bagOp{code: int(wrap.Code), pushed: wrap.Pushed, count: n}, time.Since(t)
}

func (g *gmClient) reload(table string) (reloadRsp, time.Duration) {
	raw, _ := json.Marshal(map[string]any{"tableName": table})
	t := time.Now()
	req, err := http.NewRequest(http.MethodPost, "http://"+g.base+"/gm/config/reload", bytes.NewReader(raw))
	if err != nil {
		return reloadRsp{code: -2}, time.Since(t)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GM-Token", g.token)
	resp, err := g.hc.Do(req)
	if err != nil {
		return reloadRsp{code: -1}, time.Since(t)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var wrap struct {
		Code    int32  `json:"code"`
		Version string `json:"version"`
		Tables  int64  `json:"tables"`
	}
	_ = json.Unmarshal(b, &wrap)
	if wrap.Code == 0 && resp.StatusCode >= 400 {
		return reloadRsp{code: resp.StatusCode}, time.Since(t)
	}
	return reloadRsp{code: int(wrap.Code), version: wrap.Version, tables: wrap.Tables}, time.Since(t)
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
			c.On("onBagChange", s.onBag)
			if err := c.ConnectToWS(addr, ""); err != nil {
				codes[i] = -1
				return
			}
			s.c = c
			nick := fmt.Sprintf("%s%d-%d", tag, stamp, i)
			var issued protocol.IssueTokenResponse
			code, err := call(c, "gate.user.issueToken", &protocol.IssueTokenRequest{
				Nickname: nick, Password: "123456", DeviceId: "lt14",
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
				AccessToken: issued.AccessToken, ServerId: 10001, DeviceId: "lt14",
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
				Name: fmt.Sprintf("n%d-%d", stamp%1000000, i),
			}, &created)
			if err != nil || created.Player == nil {
				if code == 0 {
					code = -3
				}
				codes[i] = code
				return
			}
			s.playerID = created.Player.PlayerId
			var entered protocol.EnterGameResponse
			code, err = call(c, "game.player.enter", &protocol.EnterGameRequest{PlayerId: s.playerID}, &entered)
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

func countBool(vals []bool) int {
	n := 0
	for _, v := range vals {
		if v {
			n++
		}
	}
	return n
}

func countN(vals []int32, want int32) int {
	n := 0
	for _, v := range vals {
		if v == want {
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
