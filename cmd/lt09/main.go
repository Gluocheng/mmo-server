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
	loadN    = 50
	potionID = int32(1001)
	bagType  = int32(2)
	arenaID  = int32(2)
	stunID   = int32(3)
)

var addr string

type slot struct {
	c      *pomeloClient.Client
	uid    int64
	potion int32
	mu     sync.Mutex
	frames []*protocol.CombatFrame
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
	fmt.Println("== 全链路 2 人禁手、作废已入队技能、喝药不扣 ==")
	slots := make([]*slot, 2)
	if !enterMain(slots, "fc") || !switchArena(slots) {
		closeAll(slots)
		return false
	}
	a, b := slots[0], slots[1]
	code, lat := addPotion(b, 2)
	fmt.Printf("发药 code=%d 耗时=%s\n", code, lat.Round(time.Millisecond))
	ok := code == 0 && potionCount(b) == 2
	dropped := false
	for attempt := 1; attempt <= 3 && !dropped; attempt++ {
		a.clearFrames()
		codeA, _ := cast(a, 4, b.uid)
		codeB, latB := cast(b, 1, a.uid)
		time.Sleep(400 * time.Millisecond)
		hit := a.sawSkillDamage(1)
		fmt.Printf("第%d次 禁手=%d 入队普攻=%d 耗时=%s 普攻打中=%v\n", attempt, codeA, codeB, latB.Round(time.Millisecond), hit)
		dropped = codeA == 0 && codeB == 0 && !hit
		if !dropped {
			time.Sleep(5200 * time.Millisecond)
		}
	}
	ok = ok && dropped
	code, lat = cast(b, 1, a.uid)
	fmt.Printf("禁手中普攻 code=%d 耗时=%s\n", code, lat.Round(time.Millisecond))
	ok = ok && code == 40055
	code, lat = usePotion(b)
	left := potionCount(b)
	fmt.Printf("禁手中喝药 code=%d 剩余=%d 耗时=%s\n", code, left, lat.Round(time.Millisecond))
	ok = ok && code == 40055 && left == 2
	code, lat = moveTo(b, 1, 0, 0)
	fmt.Printf("禁手中移动 code=%d 耗时=%s\n", code, lat.Round(time.Millisecond))
	ok = ok && code == 0
	time.Sleep(2200 * time.Millisecond)
	a.clearFrames()
	code, lat = cast(b, 1, a.uid)
	time.Sleep(400 * time.Millisecond)
	fmt.Printf("到期后普攻 code=%d 耗时=%s 打中=%v\n", code, lat.Round(time.Millisecond), a.sawSkillDamage(1))
	ok = ok && code == 0 && a.sawSkillDamage(1)
	closeAll(slots)
	time.Sleep(5 * time.Second)
	if ok {
		fmt.Println("全链路通过")
	}
	return ok
}

func load() bool {
	fmt.Printf("== 压测 %d 人互相禁手 ==\n", loadN)
	slots := make([]*slot, loadN)
	if !enterMain(slots, "ld") || !switchArena(slots) {
		closeAll(slots)
		return false
	}
	codes := make([]int, loadN)
	lats := make([]time.Duration, loadN)
	start := time.Now()
	for i := range slots {
		code, lat := addPotion(slots[i], 1)
		codes[i] = code
		lats[i] = lat
	}
	printStats("add", time.Since(start), codes, lats)
	ok := count(codes, 0) == loadN

	for _, s := range slots {
		s.clearFrames()
	}
	// 偶数点奇数。围成一圈同时点时，先被禁手的人自己那下会被丢掉。
	evens := half(slots, true)
	odds := half(slots, false)
	var wg sync.WaitGroup
	start = time.Now()
	for i := range evens {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			code, lat := cast(evens[i], 4, odds[i].uid)
			codes[i] = code
			lats[i] = lat
		}(i)
	}
	wg.Wait()
	printStats("stun", time.Since(start), codes[:len(evens)], lats[:len(evens)])
	ok = ok && count(codes[:len(evens)], 0) == len(evens)
	start = time.Now()
	for i := range odds {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			code, lat := cast(odds[i], 1, evens[i].uid)
			codes[i] = code
			lats[i] = lat
		}(i)
	}
	wg.Wait()
	printStats("queued", time.Since(start), codes[:len(odds)], lats[:len(odds)])
	ok = ok && count(codes[:len(odds)], 0) == len(odds)
	gotStun := make([]bool, len(odds))
	gotHurt := make([]bool, len(odds))
	for i := range odds {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			gotStun[i] = odds[i].waitBuff(stunID, 1500*time.Millisecond)
			gotHurt[i] = evens[i].sawSkillDamage(1)
		}(i)
	}
	wg.Wait()
	stunned := countBool(gotStun)
	hurt := countBool(gotHurt)
	fmt.Printf("被禁手=%d 入队普攻打中=%d\n", stunned, hurt)
	ok = ok && stunned == len(odds) && hurt == 0

	start = time.Now()
	for i := range odds {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			code, lat := cast(odds[i], 1, evens[i].uid)
			codes[i] = code
			lats[i] = lat
		}(i)
	}
	wg.Wait()
	printStats("cast-stunned", time.Since(start), codes[:len(odds)], lats[:len(odds)])
	ok = ok && count(codes[:len(odds)], 40055) == len(odds)

	start = time.Now()
	for i := range odds {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			code, lat := usePotion(odds[i])
			codes[i] = code
			lats[i] = lat
		}(i)
	}
	wg.Wait()
	printStats("use-stunned", time.Since(start), codes[:len(odds)], lats[:len(odds)])
	ok = ok && count(codes[:len(odds)], 40055) == len(odds)
	kept := 0
	for _, s := range odds {
		if potionCount(s) == 1 {
			kept++
		}
	}
	fmt.Printf("药水仍是1瓶=%d\n", kept)
	ok = ok && kept == len(odds)

	start = time.Now()
	for i := range odds {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			code, lat := moveTo(odds[i], 1, 0, 0)
			codes[i] = code
			lats[i] = lat
		}(i)
	}
	wg.Wait()
	printStats("move", time.Since(start), codes[:len(odds)], lats[:len(odds)])
	ok = ok && count(codes[:len(odds)], 0) == len(odds)

	fmt.Println("等禁手到期 2.2 秒")
	time.Sleep(2200 * time.Millisecond)
	start = time.Now()
	for i := range odds {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			code, lat := cast(odds[i], 1, evens[i].uid)
			codes[i] = code
			lats[i] = lat
		}(i)
	}
	wg.Wait()
	printStats("cast-after", time.Since(start), codes[:len(odds)], lats[:len(odds)])
	ok = ok && count(codes[:len(odds)], 0) == len(odds)
	closeAll(slots)
	if ok {
		fmt.Println("压测通过")
	}
	return ok
}

func half(slots []*slot, even bool) []*slot {
	out := make([]*slot, 0, len(slots)/2)
	for i, s := range slots {
		if i%2 == 0 == even {
			out = append(out, s)
		}
	}
	return out
}

func cast(s *slot, skill int32, target int64) (int, time.Duration) {
	if s == nil || s.c == nil {
		return -1, 0
	}
	t := time.Now()
	code, err := call(s.c, "game.combat.cast", &protocol.CombatCastRequest{SkillId: skill, TargetUid: target}, &protocol.None{})
	if err != nil && code == 0 {
		code = -2
	}
	return code, time.Since(t)
}

func moveTo(s *slot, x, y, z float32) (int, time.Duration) {
	t := time.Now()
	code, err := call(s.c, "game.player.move", &protocol.MoveRequest{X: x, Y: y, Z: z}, &protocol.None{})
	if err != nil && code == 0 {
		code = -2
	}
	return code, time.Since(t)
}

func addPotion(s *slot, n int32) (int, time.Duration) {
	var rsp protocol.BagListResponse
	t := time.Now()
	code, err := call(s.c, "game.bag.add", &protocol.BagAddRequest{ItemId: potionID, Count: n}, &rsp)
	if err != nil && code == 0 {
		code = -2
	}
	s.potion, _ = findPotion(&rsp)
	return code, time.Since(t)
}

func usePotion(s *slot) (int, time.Duration) {
	var rsp protocol.BagListResponse
	t := time.Now()
	code, err := call(s.c, "game.bag.use", &protocol.BagUseRequest{BagType: bagType, Slot: s.potion}, &rsp)
	if err != nil && code == 0 {
		code = -2
	}
	return code, time.Since(t)
}

func potionCount(s *slot) int32 {
	var rsp protocol.BagListResponse
	code, err := call(s.c, "game.bag.list", &protocol.BagListRequest{BagType: bagType}, &rsp)
	if err != nil || code != 0 {
		return -1
	}
	_, count := findPotion(&rsp)
	return count
}

func findPotion(rsp *protocol.BagListResponse) (slot, count int32) {
	if rsp == nil {
		return 0, 0
	}
	for _, it := range rsp.Items {
		if it != nil && it.ItemId == potionID {
			return it.Slot, it.Count
		}
	}
	return 0, 0
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
			c.On("onCombatFrame", func(msg *pomeloMessage.Message) {
				var frame protocol.CombatFrame
				if len(msg.Data) == 0 || c.Serializer().Unmarshal(msg.Data, &frame) != nil {
					return
				}
				s.mu.Lock()
				s.frames = append(s.frames, &frame)
				s.mu.Unlock()
			})
			var issued protocol.IssueTokenResponse
			code, err := call(c, "gate.user.issueToken", &protocol.IssueTokenRequest{
				Nickname: fmt.Sprintf("%s%d-%d", tag, stamp, i),
				Password: "123456",
				DeviceId: "lt09",
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
				AccessToken: issued.AccessToken, ServerId: 10001, DeviceId: "lt09",
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

func switchArena(slots []*slot) bool {
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
			code, err := call(slots[i].c, "game.player.switchScene", &protocol.SceneSwitchRequest{SceneId: arenaID}, &rsp)
			lats[i] = time.Since(t)
			if err != nil || rsp.SceneId != arenaID || rsp.Line != 1 {
				if code == 0 {
					code = -5
				}
				codes[i] = code
				return
			}
			codes[i] = 0
		}(i)
	}
	wg.Wait()
	printStats("switch-arena", time.Since(start), codes, lats)
	return count(codes, 0) == n
}

func (s *slot) sawSkillDamage(skill int32) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, f := range s.frames {
		for _, h := range f.GetHits() {
			if h != nil && h.SkillId == skill && h.Amount > 0 && h.TargetUid == s.uid {
				return true
			}
		}
	}
	return false
}

func (s *slot) waitBuff(id int32, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for {
		if s.hasBuff(id) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (s *slot) hasBuff(id int32) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, f := range s.frames {
		for _, b := range f.GetBuffs() {
			if b != nil && b.Id == id && b.Stacks > 0 && b.Uid == s.uid {
				return true
			}
		}
	}
	return false
}

func (s *slot) clearFrames() {
	s.mu.Lock()
	s.frames = nil
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
	var timed []time.Duration
	if lats != nil {
		major := 0
		best := -1
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

func countBool(flags []bool) int {
	n := 0
	for _, ok := range flags {
		if ok {
			n++
		}
	}
	return n
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
