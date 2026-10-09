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

const (
	cityN    = 201
	sceneN   = 50
	switchN  = 51
	sceneCap = 50
)

var addr string

type slot struct {
	c        *pomeloClient.Client
	playerID int64
	line     int32
	sceneID  int32
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
	fmt.Println("== 全链路 2 人看地图并切图 ==")
	slots := make([]*slot, 2)
	if !enterAll(slots, "fc") {
		closeAll(slots)
		return false
	}
	ok := true
	for i, s := range slots {
		scenes, code, lat := listScenes(s)
		fmt.Printf("scenes[%d] code=%d 耗时=%s 地图=%s\n", i, code, lat.Round(time.Millisecond), sceneNames(scenes))
		if code != 0 || !hasScenes(scenes, 1, 2, 3) {
			ok = false
		}
	}
	code, lat, view := switchTo(slots[0], 2)
	fmt.Printf("切演武场 code=%d 耗时=%s scene=%d line=%d pos=(%.0f,%.0f,%.0f)\n", code, lat.Round(time.Millisecond), view.SceneId, view.Line, view.X, view.Y, view.Z)
	if code != 0 || view.SceneId != 2 || view.Line != 1 {
		ok = false
	}
	code, lat, view = switchTo(slots[1], 3)
	fmt.Printf("切荒野 code=%d 耗时=%s scene=%d line=%d pos=(%.0f,%.0f,%.0f)\n", code, lat.Round(time.Millisecond), view.SceneId, view.Line, view.X, view.Y, view.Z)
	if code != 0 || view.SceneId != 3 || view.Line != 1 || view.X != 10 || view.Z != 10 {
		ok = false
	}
	code, lat, view = switchTo(slots[0], 3)
	fmt.Printf("离开演武场去荒野 code=%d 耗时=%s scene=%d\n", code, lat.Round(time.Millisecond), view.SceneId)
	if code != 0 || view.SceneId != 3 {
		ok = false
	}
	code, lat, _ = switchTo(slots[0], 1)
	fmt.Printf("刚离开演武场再切回主城 code=%d 耗时=%s\n", code, lat.Round(time.Millisecond))
	if code != 40053 {
		ok = false
	}
	closeAll(slots)
	if ok {
		fmt.Println("全链路通过")
	}
	return ok
}

func load() bool {
	fmt.Printf("== 压测 %d 人进主城 ==\n", cityN)
	slots := make([]*slot, cityN)
	if !enterAll(slots, "ld") {
		closeAll(slots)
		return false
	}
	fmt.Printf("进场分线 %s\n", formatLines(slots))
	line1, line2 := 0, 0
	for _, s := range slots {
		if s.line == 1 {
			line1++
		}
		if s.line == 2 {
			line2++
		}
	}
	opened := line1 == 200 && line2 == 1
	if !opened {
		fmt.Printf("主城开新线不符合预期 1线=%d 2线=%d\n", line1, line2)
	}

	fmt.Printf("== %d 人同时拉地图列表 ==\n", sceneN)
	codes := make([]int, sceneN)
	lats := make([]time.Duration, sceneN)
	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < sceneN; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			scenes, code, lat := listScenes(slots[i])
			lats[i] = lat
			if code == 0 && !hasScenes(scenes, 1, 2, 3) {
				code = -4
			}
			codes[i] = code
		}(i)
	}
	wg.Wait()
	printStats("scenes", time.Since(start), codes, lats)
	scenesOK := count(codes, 0) == sceneN

	fmt.Printf("== %d 人同时切演武场 ==\n", switchN)
	arenaOK, arenaSucc := switchMany(slots[sceneN:sceneN+switchN], 2)
	fmt.Printf("== %d 人同时切荒野 ==\n", switchN)
	wildOK, wildSucc := switchMany(slots[sceneN+switchN:sceneN+2*switchN], 3)
	fmt.Printf("演武场成功=%d 荒野成功=%d 上限=%d\n", arenaSucc, wildSucc, sceneCap)
	closeAll(slots)
	return opened && scenesOK && arenaOK && wildOK && arenaSucc == sceneCap && wildSucc == sceneCap
}

func switchMany(group []*slot, sceneID int32) (bool, int) {
	codes := make([]int, len(group))
	lats := make([]time.Duration, len(group))
	var wg sync.WaitGroup
	start := time.Now()
	for i := range group {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			code, lat, view := switchTo(group[i], sceneID)
			lats[i] = lat
			if code == 0 && (view.SceneId != sceneID || view.Line != 1) {
				code = -5
			}
			codes[i] = code
		}(i)
	}
	wg.Wait()
	printStats("switch", time.Since(start), codes, lats)
	succ := count(codes, 0)
	full := count(codes, 40051)
	return succ+full == len(group) && full >= 1, succ
}

func enterAll(slots []*slot, tag string) bool {
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
			var issued protocol.IssueTokenResponse
			code, err := call(c, "gate.user.issueToken", &protocol.IssueTokenRequest{
				Nickname: fmt.Sprintf("%s%d-%d", tag, stamp, i),
				Password: "123456",
				DeviceId: "lt06",
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
				AccessToken: issued.AccessToken, ServerId: 10001, DeviceId: "lt06",
			}, &login)
			if err != nil || login.Uid < 1 {
				if code == 0 {
					code = -3
				}
				codes[i] = code
				return
			}
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
			s.playerID = created.Player.PlayerId
			var entered protocol.EnterGameResponse
			code, err = call(c, "game.player.enter", &protocol.EnterGameRequest{
				PlayerId: s.playerID, SceneId: 1,
			}, &entered)
			if err != nil || entered.SceneId != 1 || entered.Line < 1 {
				if code == 0 {
					code = -5
				}
				codes[i] = code
				return
			}
			s.line = entered.Line
			s.sceneID = entered.SceneId
		}(i)
	}
	wg.Wait()
	printStats("enter", time.Since(start), codes, nil)
	return count(codes, 0) == n
}

func listScenes(s *slot) (*protocol.SceneListResponse, int, time.Duration) {
	if s == nil || s.c == nil {
		return nil, -1, 0
	}
	var rsp protocol.SceneListResponse
	start := time.Now()
	code, err := call(s.c, "game.player.scenes", &protocol.None{}, &rsp)
	if err != nil && code == 0 {
		code = -2
	}
	return &rsp, code, time.Since(start)
}

func switchTo(s *slot, sceneID int32) (int, time.Duration, protocol.SceneSwitchResponse) {
	var rsp protocol.SceneSwitchResponse
	if s == nil || s.c == nil {
		return -1, 0, rsp
	}
	start := time.Now()
	code, err := call(s.c, "game.player.switchScene", &protocol.SceneSwitchRequest{SceneId: sceneID}, &rsp)
	if err != nil && code == 0 {
		code = -2
	}
	return code, time.Since(start), rsp
}

func hasScenes(rsp *protocol.SceneListResponse, ids ...int32) bool {
	if rsp == nil {
		return false
	}
	got := map[int32]bool{}
	for _, sc := range rsp.Scenes {
		if sc != nil {
			got[sc.SceneId] = true
		}
	}
	for _, id := range ids {
		if !got[id] {
			return false
		}
	}
	return true
}

func sceneNames(rsp *protocol.SceneListResponse) string {
	if rsp == nil {
		return ""
	}
	parts := make([]string, 0, len(rsp.Scenes))
	for _, sc := range rsp.Scenes {
		if sc != nil {
			parts = append(parts, fmt.Sprintf("%d:%s", sc.SceneId, sc.Name))
		}
	}
	return strings.Join(parts, " ")
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
