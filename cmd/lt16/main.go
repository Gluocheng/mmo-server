package main

import (
	"flag"
	"fmt"
	"math"
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
	c       *pomeloClient.Client
	uid     int64
	line    int32
	wolf    int64
	mu      sync.Mutex
	invite  *protocol.PartyInvite
	settles []*protocol.KillSettle
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
	fmt.Println("== 全链路 2 人组队打死世界BOSS图的狼 ==")
	slots := make([]*slot, 2)
	if !enterMain(slots, "fc") {
		closeAll(slots)
		return false
	}
	var listed protocol.SceneListResponse
	code, _ := call(slots[0].c, "game.player.scenes", &protocol.None{}, &listed)
	hasBoss := false
	for _, sc := range listed.Scenes {
		if sc != nil && sc.SceneId == 4 && sc.Name == "世界BOSS" {
			hasBoss = true
		}
	}
	fmt.Printf("地图列表含世界BOSS=%v code=%d\n", hasBoss, code)
	ok := code == 0 && hasBoss
	if !switchBoss(slots) {
		closeAll(slots)
		return false
	}
	for _, s := range slots {
		if s.line != 1 || s.wolf < 1 {
			fmt.Printf("uid=%d 线=%d 狼=%d\n", s.uid, s.line, s.wolf)
			ok = false
		}
	}
	st, code, lat := partyCreate(slots[0])
	fmt.Printf("创建 code=%d 耗时=%s\n", code, lat.Round(time.Millisecond))
	ok = ok && code == 0 && st.GetPartyId() != 0
	code, _ = partyInvite(slots[0], slots[1].uid)
	ok = ok && code == 0
	var inv *protocol.PartyInvite
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		inv = slots[1].lastInvite()
		if inv != nil && inv.InviteId > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	ans, code, _ := partyAnswer(slots[1], inv.GetInviteId(), true)
	fmt.Printf("接受 code=%d 人数=%d\n", code, len(ans.GetMembers()))
	ok = ok && code == 0 && len(ans.GetMembers()) == 2
	for _, s := range slots {
		code, lat = moveTo(s, 6, 0, 6)
		fmt.Printf("uid=%d 移动 code=%d 耗时=%s\n", s.uid, code, lat.Round(time.Millisecond))
		ok = ok && code == 0
	}
	killed := false
	for n := 0; n < 20 && !killed; n++ {
		for _, s := range slots {
			code, _ = cast(s, s.wolf)
			if code != 0 && code != 40044 {
				fmt.Printf("uid=%d 施法 code=%d\n", s.uid, code)
			}
		}
		time.Sleep(1100 * time.Millisecond)
		killed = slots[0].sawClass("party") && slots[1].sawClass("party")
	}
	fmt.Printf("结算 甲=%s 乙=%s\n", slots[0].settleSummary(), slots[1].settleSummary())
	ok = ok && killed
	closeAll(slots)
	time.Sleep(1500 * time.Millisecond)
	if ok {
		fmt.Println("全链路通过")
	}
	return ok
}

func load() bool {
	fmt.Printf("== 压测 %d 人进世界BOSS图并点名狼 ==\n", loadN)
	fmt.Println("等狼复活 16 秒")
	time.Sleep(16 * time.Second)
	slots := make([]*slot, loadN)
	if !enterMain(slots, "ld") {
		closeAll(slots)
		return false
	}
	if !switchBoss(slots) {
		closeAll(slots)
		return false
	}
	line1, line2, wolves := 0, 0, 0
	for _, s := range slots {
		if s.line == 1 {
			line1++
		}
		if s.line == 2 {
			line2++
		}
		if s.wolf > 0 {
			wolves++
		}
	}
	fmt.Printf("分线 1=%d 2=%d 看到狼=%d\n", line1, line2, wolves)
	ok := line1 == 30 && line2 == 20 && wolves == loadN

	codes := make([]int, loadN)
	lats := make([]time.Duration, loadN)
	var wg sync.WaitGroup
	start := time.Now()
	for i := range slots {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			code, lat := moveTo(slots[i], 6, 0, 6)
			lats[i] = lat
			codes[i] = code
		}(i)
	}
	wg.Wait()
	printStats("move", time.Since(start), codes, lats)
	ok = ok && count(codes, 0) == loadN

	start = time.Now()
	for i := range slots {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			code, lat := cast(slots[i], slots[i].wolf)
			lats[i] = lat
			codes[i] = code
		}(i)
	}
	wg.Wait()
	printStats("cast", time.Since(start), codes, lats)
	ok = ok && count(codes, 0) == loadN
	closeAll(slots)
	if ok {
		fmt.Println("压测通过")
	}
	return ok
}

func switchBoss(slots []*slot) bool {
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
			code, err := call(slots[i].c, "game.player.switchScene", &protocol.SceneSwitchRequest{SceneId: 4}, &rsp)
			lats[i] = time.Since(t)
			if err != nil || rsp.SceneId != 4 || rsp.Line < 1 {
				if code == 0 {
					code = -5
				}
				codes[i] = code
				return
			}
			slots[i].line = rsp.Line
			slots[i].wolf = nearestWolf(rsp.Nearby, 6, 6)
			if slots[i].wolf < 1 {
				codes[i] = -8
				return
			}
			codes[i] = 0
		}(i)
	}
	wg.Wait()
	printStats("switch-boss", time.Since(start), codes, lats)
	return count(codes, 0) == n
}

func nearestWolf(nearby []*protocol.SceneActor, x, z float32) int64 {
	var best int64
	bestD := float32(math.MaxFloat32)
	for _, n := range nearby {
		if n == nil || n.ActorType != 1 || n.ConfigId != 3 {
			continue
		}
		dx, dz := n.X-x, n.Z-z
		d := dx*dx + dz*dz
		if d < bestD {
			bestD = d
			best = n.Uid
		}
	}
	return best
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
			c.On("onPartyInvite", func(msg *pomeloMessage.Message) {
				var inv protocol.PartyInvite
				if len(msg.Data) == 0 || c.Serializer().Unmarshal(msg.Data, &inv) != nil {
					return
				}
				s.mu.Lock()
				s.invite = &inv
				s.mu.Unlock()
			})
			c.On("onKillSettle", func(msg *pomeloMessage.Message) {
				var st protocol.KillSettle
				if len(msg.Data) == 0 || c.Serializer().Unmarshal(msg.Data, &st) != nil {
					return
				}
				s.mu.Lock()
				s.settles = append(s.settles, &st)
				s.mu.Unlock()
			})
			var issued protocol.IssueTokenResponse
			code, err := call(c, "gate.user.issueToken", &protocol.IssueTokenRequest{
				Nickname: fmt.Sprintf("%s%d-%d", tag, stamp, i),
				Password: "123456",
				DeviceId: "lt16",
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
				AccessToken: issued.AccessToken, ServerId: 10001, DeviceId: "lt16",
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
				Name: fmt.Sprintf("b%d-%d", stamp%1000000, i),
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

func moveTo(s *slot, x, y, z float32) (int, time.Duration) {
	if s == nil || s.c == nil {
		return -1, 0
	}
	t := time.Now()
	code, err := call(s.c, "game.player.move", &protocol.MoveRequest{X: x, Y: y, Z: z}, &protocol.None{})
	if err != nil && code == 0 {
		code = -2
	}
	return code, time.Since(t)
}

func cast(s *slot, target int64) (int, time.Duration) {
	if s == nil || s.c == nil {
		return -1, 0
	}
	t := time.Now()
	code, err := call(s.c, "game.combat.cast", &protocol.CombatCastRequest{SkillId: 1, TargetUid: target}, &protocol.None{})
	if err != nil && code == 0 {
		code = -2
	}
	return code, time.Since(t)
}

func partyCreate(s *slot) (*protocol.PartyState, int, time.Duration) {
	var st protocol.PartyState
	t := time.Now()
	code, err := call(s.c, "game.party.create", &protocol.None{}, &st)
	if err != nil && code == 0 {
		code = -2
	}
	return &st, code, time.Since(t)
}

func partyInvite(s *slot, target int64) (int, time.Duration) {
	t := time.Now()
	code, err := call(s.c, "game.party.invite", &protocol.PartyInviteRequest{TargetUid: target}, &protocol.None{})
	if err != nil && code == 0 {
		code = -2
	}
	return code, time.Since(t)
}

func partyAnswer(s *slot, inviteID int64, accept bool) (*protocol.PartyState, int, time.Duration) {
	var st protocol.PartyState
	t := time.Now()
	code, err := call(s.c, "game.party.answer", &protocol.PartyAnswerRequest{InviteId: inviteID, Accept: accept}, &st)
	if err != nil && code == 0 {
		code = -2
	}
	return &st, code, time.Since(t)
}

func (s *slot) lastInvite() *protocol.PartyInvite {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.invite
}

func (s *slot) sawClass(class string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, st := range s.settles {
		if st != nil && st.Class == class && st.MonsterId == 3 {
			return true
		}
	}
	return false
}

func (s *slot) settleSummary() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.settles) == 0 {
		return "无"
	}
	st := s.settles[len(s.settles)-1]
	return fmt.Sprintf("class=%s monster=%d rank=%d items=%d", st.Class, st.MonsterId, st.Rank, len(st.Items))
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
