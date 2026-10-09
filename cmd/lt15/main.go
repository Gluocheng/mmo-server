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
	c      *pomeloClient.Client
	uid    int64
	mu     sync.Mutex
	invite *protocol.PartyInvite
	states []*protocol.PartyState
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
	fmt.Println("== 全链路 2 人组队 ==")
	slots := make([]*slot, 2)
	if !enterMain(slots, "fc") {
		closeAll(slots)
		return false
	}
	a, b := slots[0], slots[1]
	st, code, lat := partyCreate(a)
	fmt.Printf("创建 code=%d 耗时=%s 队长=%d 人数=%d\n", code, lat.Round(time.Millisecond), st.GetLeaderUid(), len(st.GetMembers()))
	ok := code == 0 && st.GetLeaderUid() == a.uid && len(st.GetMembers()) == 1
	code, lat = partyInvite(a, b.uid)
	fmt.Printf("邀请 code=%d 耗时=%s\n", code, lat.Round(time.Millisecond))
	ok = ok && code == 0
	deadline := time.Now().Add(2 * time.Second)
	var inv *protocol.PartyInvite
	for time.Now().Before(deadline) {
		inv = b.lastInvite()
		if inv != nil && inv.InviteId > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	fmt.Printf("被邀请者收到邀请=%v id=%d\n", inv != nil && inv.InviteId > 0, inv.GetInviteId())
	ok = ok && inv != nil && inv.InviteId > 0 && inv.LeaderUid == a.uid
	ans, code, lat := partyAnswer(b, inv.GetInviteId(), true)
	fmt.Printf("接受 code=%d 耗时=%s 人数=%d\n", code, lat.Round(time.Millisecond), len(ans.GetMembers()))
	ok = ok && code == 0 && len(ans.GetMembers()) == 2
	time.Sleep(300 * time.Millisecond)
	fmt.Printf("双方推送人数 队长=%d 队员=%d\n", a.memberCount(), b.memberCount())
	ok = ok && a.memberCount() == 2 && b.memberCount() == 2
	code, lat = partyLeave(b)
	fmt.Printf("离队 code=%d 耗时=%s\n", code, lat.Round(time.Millisecond))
	ok = ok && code == 0
	time.Sleep(300 * time.Millisecond)
	empty := a.sawEmpty()
	fmt.Printf("队长收到空名单=%v\n", empty)
	ok = ok && empty
	closeAll(slots)
	time.Sleep(500 * time.Millisecond)
	if ok {
		fmt.Println("全链路通过")
	}
	return ok
}

func load() bool {
	fmt.Printf("== 压测 %d 人组队 ==\n", loadN)
	slots := make([]*slot, loadN)
	if !enterMain(slots, "ld") {
		closeAll(slots)
		return false
	}
	half := loadN / 2
	codes := make([]int, half)
	lats := make([]time.Duration, half)
	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < half; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			st, code, lat := partyCreate(slots[i])
			lats[i] = lat
			if code == 0 && (st.GetLeaderUid() != slots[i].uid || len(st.GetMembers()) != 1) {
				code = -6
			}
			codes[i] = code
		}(i)
	}
	wg.Wait()
	printStats("create", time.Since(start), codes, lats)
	ok := count(codes, 0) == half

	start = time.Now()
	for i := 0; i < half; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			code, lat := partyInvite(slots[i], slots[i+half].uid)
			lats[i] = lat
			codes[i] = code
		}(i)
	}
	wg.Wait()
	printStats("invite", time.Since(start), codes, lats)
	ok = ok && count(codes, 0) == half

	time.Sleep(500 * time.Millisecond)
	gotInvite := 0
	for i := half; i < loadN; i++ {
		if inv := slots[i].lastInvite(); inv != nil && inv.InviteId > 0 {
			gotInvite++
		}
	}
	fmt.Printf("收到邀请=%d/%d\n", gotInvite, half)
	ok = ok && gotInvite == half

	start = time.Now()
	for i := 0; i < half; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			inv := slots[i+half].lastInvite()
			st, code, lat := partyAnswer(slots[i+half], inv.GetInviteId(), true)
			lats[i] = lat
			if code == 0 && len(st.GetMembers()) != 2 {
				code = -7
			}
			codes[i] = code
		}(i)
	}
	wg.Wait()
	printStats("answer", time.Since(start), codes, lats)
	ok = ok && count(codes, 0) == half
	closeAll(slots)
	if ok {
		fmt.Println("压测通过")
	}
	return ok
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
			c.On("onParty", func(msg *pomeloMessage.Message) {
				var st protocol.PartyState
				if len(msg.Data) > 0 && c.Serializer().Unmarshal(msg.Data, &st) != nil {
					return
				}
				s.mu.Lock()
				s.states = append(s.states, &st)
				s.mu.Unlock()
			})
			var issued protocol.IssueTokenResponse
			code, err := call(c, "gate.user.issueToken", &protocol.IssueTokenRequest{
				Nickname: fmt.Sprintf("%s%d-%d", tag, stamp, i),
				Password: "123456",
				DeviceId: "lt15",
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
				AccessToken: issued.AccessToken, ServerId: 10001, DeviceId: "lt15",
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
				Name: fmt.Sprintf("p%d-%d", stamp%1000000, i),
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

func partyCreate(s *slot) (*protocol.PartyState, int, time.Duration) {
	var st protocol.PartyState
	if s == nil || s.c == nil {
		return &st, -1, 0
	}
	t := time.Now()
	code, err := call(s.c, "game.party.create", &protocol.None{}, &st)
	if err != nil && code == 0 {
		code = -2
	}
	return &st, code, time.Since(t)
}

func partyInvite(s *slot, target int64) (int, time.Duration) {
	if s == nil || s.c == nil {
		return -1, 0
	}
	t := time.Now()
	code, err := call(s.c, "game.party.invite", &protocol.PartyInviteRequest{TargetUid: target}, &protocol.None{})
	if err != nil && code == 0 {
		code = -2
	}
	return code, time.Since(t)
}

func partyAnswer(s *slot, inviteID int64, accept bool) (*protocol.PartyState, int, time.Duration) {
	var st protocol.PartyState
	if s == nil || s.c == nil {
		return &st, -1, 0
	}
	t := time.Now()
	code, err := call(s.c, "game.party.answer", &protocol.PartyAnswerRequest{InviteId: inviteID, Accept: accept}, &st)
	if err != nil && code == 0 {
		code = -2
	}
	return &st, code, time.Since(t)
}

func partyLeave(s *slot) (int, time.Duration) {
	if s == nil || s.c == nil {
		return -1, 0
	}
	t := time.Now()
	code, err := call(s.c, "game.party.leave", &protocol.None{}, &protocol.None{})
	if err != nil && code == 0 {
		code = -2
	}
	return code, time.Since(t)
}

func (s *slot) lastInvite() *protocol.PartyInvite {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.invite
}

func (s *slot) memberCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, st := range s.states {
		if st != nil && len(st.Members) > n {
			n = len(st.Members)
		}
	}
	return n
}

func (s *slot) sawEmpty() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, st := range s.states {
		if st != nil && st.PartyId == 0 {
			return true
		}
	}
	return false
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
