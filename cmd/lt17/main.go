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
	c      *pomeloClient.Client
	uid    int64
	line   int32
	wolf   int64
	mu     sync.Mutex
	snaps  map[int64]*protocol.CombatSnapshot
	gones  []int64
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
	fmt.Println("== 全链路 2 人荒野快照、消失、禁手到期和跳号 ==")
	slots := make([]*slot, 2)
	if !enterMain(slots, "fc") {
		closeAll(slots)
		return false
	}
	ok := true
	for _, s := range slots {
		if !waitSnap(s, s.uid, 2*time.Second) {
			fmt.Printf("uid=%d 进场后没有自己的快照\n", s.uid)
			ok = false
			continue
		}
		fmt.Printf("uid=%d 进场响应无生命字段 快照生命=%d\n", s.uid, s.snap(s.uid).GetHp())
		if s.snap(s.uid).GetHp() < 1 {
			ok = false
		}
	}
	if !switchScene(slots, 3, 1) {
		closeAll(slots)
		return false
	}
	for _, s := range slots {
		if s.wolf < 1 {
			fmt.Printf("uid=%d 附近没有野狼\n", s.uid)
			ok = false
			continue
		}
		if !waitSnap(s, s.wolf, 2*time.Second) {
			fmt.Printf("uid=%d 没有狼的快照\n", s.uid)
			ok = false
			continue
		}
		fmt.Printf("uid=%d 狼=%d 快照生命=%d\n", s.uid, s.wolf, s.snap(s.wolf).GetHp())
	}
	if !ok {
		closeAll(slots)
		return false
	}
	a, b := slots[0], slots[1]
	wolfHP := a.snap(a.wolf).GetHp()
	a.clearFrames()
	b.clearFrames()
	code, lat := cast(a, 4, b.uid)
	fmt.Printf("禁手 code=%d 耗时=%s\n", code, lat.Round(time.Millisecond))
	ok = code == 0 && b.waitBuff(3, 1, 2*time.Second)
	fmt.Printf("禁手挂上=%v\n", ok)
	removed := b.waitBuff(3, 0, 3*time.Second)
	quiet := b.frameCount()
	tick := b.maxTick()
	time.Sleep(500 * time.Millisecond)
	idle := b.frameCount() == quiet
	fmt.Printf("禁手卸下=%v 空闲半秒无帧=%v 卸下拍=%d\n", removed, idle, tick)
	ok = ok && removed && idle && b.expireWithoutDamage(3)
	code, lat = cast(a, 2, 0)
	fmt.Printf("范围斩 code=%d 耗时=%s\n", code, lat.Round(time.Millisecond))
	jumped := b.waitTickAfter(tick+1, 2*time.Second)
	fmt.Printf("下一帧拍=%d 期望大于%d\n", b.maxTick(), tick+1)
	ok = ok && code == 0 && jumped

	a.clearGone()
	b.clearGone()
	a.clearFrames()
	code, lat = moveTo(a, 80, 0, 10)
	fmt.Printf("走出视野 code=%d 耗时=%s\n", code, lat.Round(time.Millisecond))
	leftWolf := a.waitGone(a.wolf, 2*time.Second)
	leftPeer := b.waitGone(a.uid, 2*time.Second)
	fmt.Printf("自己收到狼消失=%v 对方收到自己消失=%v\n", leftWolf, leftPeer)
	ok = ok && code == 0 && leftWolf && leftPeer

	code, _ = moveTo(b, 16, 0, 10)
	ok = ok && code == 0
	b.clearFrames()
	code, lat = cast(b, 1, b.wolf)
	fmt.Printf("远处点狼 code=%d 耗时=%s\n", code, lat.Round(time.Millisecond))
	hit := b.waitHit(b.wolf, 2*time.Second)
	leaked := a.waitHit(a.wolf, 400*time.Millisecond)
	fmt.Printf("打狼者收到目标生命=%d 视野外收到=%v\n", hit, leaked > 0)
	ok = ok && code == 0 && hit > 0 && hit < wolfHP && leaked == 0

	a.clearSnaps()
	code, _ = moveTo(a, 16, 0, 10)
	back := a.waitSnap(a.wolf, 2*time.Second)
	fmt.Printf("再进视野快照生命=%d 命中生命=%d\n", a.snap(a.wolf).GetHp(), hit)
	ok = ok && code == 0 && back && a.snap(a.wolf).GetHp() == hit
	closeAll(slots)
	time.Sleep(1500 * time.Millisecond)
	if ok {
		fmt.Println("全链路通过")
	}
	return ok
}

func load() bool {
	fmt.Printf("== 压测 %d 人荒野快照、走出、走进、点名 ==\n", loadN)
	slots := make([]*slot, loadN)
	if !enterMain(slots, "ld") {
		closeAll(slots)
		return false
	}
	if !switchScene(slots, 3, 1) {
		closeAll(slots)
		return false
	}
	ok := true
	snapped := 0
	for _, s := range slots {
		if s.wolf > 0 && waitSnap(s, s.uid, 2*time.Second) && waitSnap(s, s.wolf, 2*time.Second) {
			snapped++
		}
	}
	fmt.Printf("收到自己和狼快照=%d\n", snapped)
	ok = snapped == loadN

	codes := make([]int, loadN)
	lats := make([]time.Duration, loadN)
	var wg sync.WaitGroup
	for _, s := range slots {
		s.clearGone()
	}
	start := time.Now()
	for i := range slots {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			code, lat := moveTo(slots[i], 80, 0, 10)
			codes[i] = code
			lats[i] = lat
		}(i)
	}
	wg.Wait()
	printStats("move-out", time.Since(start), codes, lats)
	ok = ok && count(codes, 0) == loadN
	gone := 0
	for _, s := range slots {
		if s.waitGone(s.wolf, 2*time.Second) {
			gone++
		}
	}
	fmt.Printf("收到狼消失=%d\n", gone)
	ok = ok && gone == loadN

	start = time.Now()
	for i := range slots {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			slots[i].clearSnaps()
			code, lat := moveTo(slots[i], 16, 0, 10)
			codes[i] = code
			lats[i] = lat
		}(i)
	}
	wg.Wait()
	printStats("move-in", time.Since(start), codes, lats)
	ok = ok && count(codes, 0) == loadN
	back := 0
	for _, s := range slots {
		if s.waitSnap(s.wolf, 2*time.Second) {
			back++
		}
	}
	fmt.Printf("再进视野收到狼快照=%d\n", back)
	ok = ok && back == loadN

	start = time.Now()
	for i := range slots {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			slots[i].clearFrames()
			code, lat := cast(slots[i], 1, slots[i].wolf)
			codes[i] = code
			lats[i] = lat
		}(i)
	}
	wg.Wait()
	printStats("cast", time.Since(start), codes, lats)
	ok = ok && count(codes, 0) == loadN
	framed := 0
	for _, s := range slots {
		if s.waitHit(s.wolf, 2*time.Second) > 0 || s.sawDead(s.wolf) {
			framed++
		}
	}
	fmt.Printf("收到打狼帧=%d\n", framed)
	ok = ok && framed == loadN

	fmt.Println("等切图冷却 12 秒")
	time.Sleep(12 * time.Second)
	if !switchScene(slots, 4, 0) {
		closeAll(slots)
		return false
	}
	line1, line2 := wolfSet(slots, 1), wolfSet(slots, 2)
	fmt.Printf("世界BOSS 1线狼=%d 2线狼=%d\n", len(line1), len(line2))
	ok = ok && len(line1) > 0 && len(line2) > 0 && disjoint(line1, line2)
	cross := 0
	for _, s := range slots {
		foreign := line1
		if s.line == 1 {
			foreign = line2
		}
		if s.sawAny(foreign) {
			cross++
		}
	}
	fmt.Printf("快照串线=%d\n", cross)
	ok = ok && cross == 0
	var hitter *slot
	for _, s := range slots {
		if s.line == 1 && s.wolf > 0 {
			hitter = s
			break
		}
	}
	if hitter == nil {
		ok = false
	} else {
		for _, s := range slots {
			s.clearFrames()
		}
		code, _ := moveTo(hitter, 6, 0, 6)
		if code == 0 {
			code, _ = cast(hitter, 1, hitter.wolf)
		}
		time.Sleep(500 * time.Millisecond)
		leaked := 0
		for _, s := range slots {
			if s.line == 2 && s.sawHit(hitter.wolf) {
				leaked++
			}
		}
		fmt.Printf("1线点名 code=%d 2线收到该狼帧=%d\n", code, leaked)
		ok = ok && code == 0 && leaked == 0
	}
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
			s := &slot{snaps: map[int64]*protocol.CombatSnapshot{}}
			slots[i] = s
			c := pomeloClient.New(pomeloClient.WithRequestTimeout(20 * time.Second))
			if err := c.ConnectToWS(addr, ""); err != nil {
				codes[i] = -1
				return
			}
			s.c = c
			c.On("onCombatSnapshot", func(msg *pomeloMessage.Message) {
				var snap protocol.CombatSnapshot
				if len(msg.Data) == 0 || c.Serializer().Unmarshal(msg.Data, &snap) != nil || snap.Uid < 1 {
					return
				}
				s.mu.Lock()
				s.snaps[snap.Uid] = &snap
				s.mu.Unlock()
			})
			c.On("onCombatGone", func(msg *pomeloMessage.Message) {
				var gone protocol.CombatGone
				if len(msg.Data) == 0 || c.Serializer().Unmarshal(msg.Data, &gone) != nil || gone.Uid < 1 {
					return
				}
				s.mu.Lock()
				s.gones = append(s.gones, gone.Uid)
				s.mu.Unlock()
			})
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
				DeviceId: "lt17",
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
				AccessToken: issued.AccessToken, ServerId: 10001, DeviceId: "lt17",
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
				Name: fmt.Sprintf("s%d-%d", stamp%1000000, i),
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

func switchScene(slots []*slot, sceneID, wolfCfg int32) bool {
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
			code, err := call(slots[i].c, "game.player.switchScene", &protocol.SceneSwitchRequest{SceneId: sceneID}, &rsp)
			lats[i] = time.Since(t)
			if err != nil || rsp.SceneId != sceneID || rsp.Line < 1 {
				if code == 0 {
					code = -5
				}
				codes[i] = code
				return
			}
			slots[i].line = rsp.Line
			slots[i].wolf = nearestMonster(rsp.Nearby, wolfCfg, rsp.X, rsp.Z)
			if wolfCfg > 0 && slots[i].wolf < 1 {
				codes[i] = -8
				return
			}
			codes[i] = 0
		}(i)
	}
	wg.Wait()
	printStats(fmt.Sprintf("switch-%d", sceneID), time.Since(start), codes, lats)
	return count(codes, 0) == n
}

func nearestMonster(nearby []*protocol.SceneActor, configID int32, x, z float32) int64 {
	var best int64
	bestD := float32(math.MaxFloat32)
	for _, n := range nearby {
		if n == nil || n.ActorType != 1 {
			continue
		}
		if configID > 0 && n.ConfigId != configID {
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

func wolfSet(slots []*slot, line int32) map[int64]struct{} {
	out := map[int64]struct{}{}
	for _, s := range slots {
		if s == nil || s.line != line {
			continue
		}
		if s.wolf > 0 {
			out[s.wolf] = struct{}{}
		}
	}
	return out
}

func disjoint(a, b map[int64]struct{}) bool {
	for id := range a {
		if _, ok := b[id]; ok {
			return false
		}
	}
	return true
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

func waitSnap(s *slot, uid int64, d time.Duration) bool {
	return s.waitSnap(uid, d)
}

func (s *slot) waitSnap(uid int64, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if s.snap(uid) != nil {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return s.snap(uid) != nil
}

func (s *slot) waitGone(uid int64, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if s.gone(uid) {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return s.gone(uid)
}

func (s *slot) waitBuff(id, stacks int32, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if s.sawBuff(id, stacks) {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return s.sawBuff(id, stacks)
}

func (s *slot) waitTickAfter(tick int64, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if s.maxTick() > tick {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return s.maxTick() > tick
}

func (s *slot) waitHit(target int64, d time.Duration) int32 {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if hp := s.hitHP(target); hp > 0 {
			return hp
		}
		time.Sleep(20 * time.Millisecond)
	}
	return s.hitHP(target)
}

func (s *slot) snap(uid int64) *protocol.CombatSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snaps[uid]
}

func (s *slot) gone(uid int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range s.gones {
		if id == uid {
			return true
		}
	}
	return false
}

func (s *slot) sawBuff(id, stacks int32) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, f := range s.frames {
		for _, b := range f.GetBuffs() {
			if b != nil && b.Id == id && b.Stacks == stacks {
				return true
			}
		}
	}
	return false
}

func (s *slot) expireWithoutDamage(id int32) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, f := range s.frames {
		for _, b := range f.GetBuffs() {
			if b == nil || b.Id != id || b.Stacks != 0 {
				continue
			}
			for _, h := range f.GetHits() {
				if h != nil && h.Amount > 0 {
					return false
				}
			}
			return true
		}
	}
	return false
}

func (s *slot) maxTick() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	var max int64
	for _, f := range s.frames {
		if f != nil && f.Tick > max {
			max = f.Tick
		}
	}
	return max
}

func (s *slot) frameCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.frames)
}

func (s *slot) hitHP(target int64) int32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	var hp int32
	for _, f := range s.frames {
		for _, h := range f.GetHits() {
			if h != nil && h.TargetUid == target && h.TargetHp > 0 {
				hp = h.TargetHp
			}
		}
	}
	return hp
}

func (s *slot) sawDead(target int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, f := range s.frames {
		for _, h := range f.GetHits() {
			if h != nil && h.TargetUid == target && h.Dead {
				return true
			}
		}
	}
	return false
}

func (s *slot) sawHit(target int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, f := range s.frames {
		for _, h := range f.GetHits() {
			if h != nil && h.TargetUid == target {
				return true
			}
		}
	}
	return false
}

func (s *slot) sawAny(ids map[int64]struct{}) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id := range s.snaps {
		if _, ok := ids[id]; ok {
			return true
		}
	}
	return false
}

func (s *slot) clearFrames() {
	s.mu.Lock()
	s.frames = nil
	s.mu.Unlock()
}

func (s *slot) clearGone() {
	s.mu.Lock()
	s.gones = nil
	s.mu.Unlock()
}

func (s *slot) clearSnaps() {
	s.mu.Lock()
	s.snaps = map[int64]*protocol.CombatSnapshot{}
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
