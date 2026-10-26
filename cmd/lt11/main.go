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
	loadN    = 50          // 压测人数
	potionID = int32(1001) // 小型生命药水 → 消耗品背包
	bagType  = int32(2)    // 消耗品背包，槽位 0..31，单格堆叠上限 99
)

var addr string

// slot 是一名压测客户端：一条网关连接 + 该玩家最新背包快照。
type slot struct {
	c   *pomeloClient.Client
	uid int64
	mu  sync.Mutex
	bag *protocol.BagListResponse
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
	fmt.Println("== 全链路 2 人背包堆叠、扣减、移动、拆分 ==")
	slots := make([]*slot, 2)
	if !enterMain(slots, "fc") {
		closeAll(slots)
		return false
	}
	ok := true

	// add 50：空背包，应堆到 slot 0，count=50
	for _, s := range slots {
		code, lat := bagAdd(s, 50)
		st := s.stacks(potionID)
		fmt.Printf("uid=%d add50 code=%d 耗时=%s 堆叠=%v\n", s.uid, code, lat.Round(time.Millisecond), fmtStacks(st))
		ok = ok && code == 0 && len(st) == 1 && st[0][0] == 0 && st[0][1] == 50
	}

	// add 60：50+60=110 > 99，应 slot0 堆满 99，剩 11 落 slot1
	for _, s := range slots {
		code, lat := bagAdd(s, 60)
		st := s.stacks(potionID)
		fmt.Printf("uid=%d add60 code=%d 耗时=%s 堆叠=%v\n", s.uid, code, lat.Round(time.Millisecond), fmtStacks(st))
		ok = ok && code == 0 && len(st) == 2 && st[0][1] == 99 && st[1][1] == 11
	}

	// list：应和当前状态一致
	for _, s := range slots {
		code, lat := bagList(s)
		fmt.Printf("uid=%d list code=%d 耗时=%s 堆叠=%v\n", s.uid, code, lat.Round(time.Millisecond), fmtStacks(s.stacks(potionID)))
		ok = ok && code == 0
	}

	// remove 按 itemId 扣 30：从 slot0 扣，slot0 99→69，slot1 不变
	for _, s := range slots {
		code, lat := bagRemove(s, 30)
		st := s.stacks(potionID)
		fmt.Printf("uid=%d remove30 code=%d 耗时=%s 堆叠=%v\n", s.uid, code, lat.Round(time.Millisecond), fmtStacks(st))
		ok = ok && code == 0 && len(st) == 2 && st[0][1] == 69 && st[1][1] == 11
	}

	// move slot1 → slot2（空槽）
	for _, s := range slots {
		code, lat := bagMove(s, 1, 2)
		st := s.stacks(potionID)
		fmt.Printf("uid=%d move1->2 code=%d 耗时=%s 堆叠=%v\n", s.uid, code, lat.Round(time.Millisecond), fmtStacks(st))
		ok = ok && code == 0 && len(st) == 2 && st[1][0] == 2 && st[1][1] == 11
	}

	// split slot0 拆 10：slot0 69→59，新堆 10 落最小空槽 slot1
	for _, s := range slots {
		code, lat := bagSplit(s, 0, 10)
		st := s.stacks(potionID)
		fmt.Printf("uid=%d split0x10 code=%d 耗时=%s 堆叠=%v\n", s.uid, code, lat.Round(time.Millisecond), fmtStacks(st))
		ok = ok && code == 0 && len(st) == 3 && st[0][1] == 59
	}

	closeAll(slots)
	time.Sleep(1500 * time.Millisecond)
	if ok {
		fmt.Println("全链路通过")
	}
	return ok
}

func load() bool {
	fmt.Printf("== 压测 %d 人背包并发读写 ==\n", loadN)
	slots := make([]*slot, loadN)
	if !enterMain(slots, "ld") {
		closeAll(slots)
		return false
	}
	ok := true

	// 阶段1：并发 add 50 到各自空背包（历史上并发 add 到空背包会死锁 MySQL 1213）
	ok = parallelBag("add50", slots, func(s *slot) (int, time.Duration) { return bagAdd(s, 50) }) && ok
	ok = checkStacks("add50 后", slots, func(st [][2]int32) bool {
		return len(st) == 1 && st[0][0] == 0 && st[0][1] == 50
	}) && ok

	// 阶段2：并发 add 60，触发跨格堆叠
	ok = parallelBag("add60", slots, func(s *slot) (int, time.Duration) { return bagAdd(s, 60) }) && ok
	ok = checkStacks("add60 后", slots, func(st [][2]int32) bool {
		return len(st) == 2 && st[0][1] == 99 && st[1][1] == 11
	}) && ok

	// 阶段3：并发 list
	ok = parallelBag("list", slots, func(s *slot) (int, time.Duration) { return bagList(s) }) && ok

	// 阶段4：并发 remove 按 itemId 扣 30
	ok = parallelBag("remove30", slots, func(s *slot) (int, time.Duration) { return bagRemove(s, 30) }) && ok
	ok = checkStacks("remove30 后", slots, func(st [][2]int32) bool {
		return len(st) == 2 && st[0][1] == 69 && st[1][1] == 11
	}) && ok

	// 阶段5：并发 move slot1 → slot2
	ok = parallelBag("move1->2", slots, func(s *slot) (int, time.Duration) { return bagMove(s, 1, 2) }) && ok
	ok = checkStacks("move 后", slots, func(st [][2]int32) bool {
		return len(st) == 2 && st[0][1] == 69 && st[1][0] == 2 && st[1][1] == 11
	}) && ok

	// 阶段6：并发 split slot0 拆 10
	ok = parallelBag("split0x10", slots, func(s *slot) (int, time.Duration) { return bagSplit(s, 0, 10) }) && ok
	// 拆分不改变总量：remove30 后 69+11=80，拆完 59+10+11 仍是 80
	ok = checkStacks("split 后", slots, func(st [][2]int32) bool {
		return len(st) == 3 && st[0][1] == 59 && total(st) == 80
	}) && ok

	closeAll(slots)
	if ok {
		fmt.Println("压测通过")
	}
	return ok
}

// parallelBag 并发对每个客户端执行一次背包操作，打印统计并返回是否全部成功。
func parallelBag(name string, slots []*slot, fn func(*slot) (int, time.Duration)) bool {
	n := len(slots)
	codes := make([]int, n)
	lats := make([]time.Duration, n)
	var wg sync.WaitGroup
	start := time.Now()
	for i := range slots {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes[i], lats[i] = fn(slots[i])
		}(i)
	}
	wg.Wait()
	printStats(name, time.Since(start), codes, lats)
	return count(codes, 0) == n
}

// checkStacks 用期望函数核对每个客户端的堆叠，返回是否全部符合。
func checkStacks(name string, slots []*slot, want func([][2]int32) bool) bool {
	bad := 0
	for _, s := range slots {
		st := s.stacks(potionID)
		if !want(st) {
			bad++
			if bad <= 3 {
				fmt.Printf("uid=%d %s 堆叠异常 %v\n", s.uid, name, fmtStacks(st))
			}
		}
	}
	fmt.Printf("%s 堆叠正确=%d 异常=%d\n", name, len(slots)-bad, bad)
	return bad == 0
}

func total(st [][2]int32) int32 {
	var t int32
	for _, p := range st {
		t += p[1]
	}
	return t
}

// ---- 背包操作：返回业务码与耗时，并把最新背包快照存回 slot ----

func bagAdd(s *slot, n int32) (int, time.Duration) {
	var rsp protocol.BagListResponse
	t := time.Now()
	code, err := call(s.c, "game.bag.add", &protocol.BagAddRequest{ItemId: potionID, Count: n}, &rsp)
	if err != nil && code == 0 {
		code = -2
	}
	s.setBag(&rsp)
	return code, time.Since(t)
}

func bagList(s *slot) (int, time.Duration) {
	var rsp protocol.BagListResponse
	t := time.Now()
	code, err := call(s.c, "game.bag.list", &protocol.BagListRequest{BagType: bagType}, &rsp)
	if err != nil && code == 0 {
		code = -2
	}
	s.setBag(&rsp)
	return code, time.Since(t)
}

func bagRemove(s *slot, n int32) (int, time.Duration) {
	var rsp protocol.BagListResponse
	t := time.Now()
	code, err := call(s.c, "game.bag.remove", &protocol.BagRemoveRequest{
		BagType: bagType, ItemId: potionID, Count: n, BySlot: false,
	}, &rsp)
	if err != nil && code == 0 {
		code = -2
	}
	s.setBag(&rsp)
	return code, time.Since(t)
}

func bagMove(s *slot, from, to int32) (int, time.Duration) {
	var rsp protocol.BagListResponse
	t := time.Now()
	code, err := call(s.c, "game.bag.move", &protocol.BagMoveRequest{
		BagType: bagType, FromSlot: from, ToSlot: to,
	}, &rsp)
	if err != nil && code == 0 {
		code = -2
	}
	s.setBag(&rsp)
	return code, time.Since(t)
}

func bagSplit(s *slot, from, n int32) (int, time.Duration) {
	var rsp protocol.BagListResponse
	t := time.Now()
	code, err := call(s.c, "game.bag.split", &protocol.BagSplitRequest{
		BagType: bagType, FromSlot: from, Count: n,
	}, &rsp)
	if err != nil && code == 0 {
		code = -2
	}
	s.setBag(&rsp)
	return code, time.Since(t)
}

func (s *slot) setBag(rsp *protocol.BagListResponse) {
	s.mu.Lock()
	s.bag = rsp
	s.mu.Unlock()
}

// stacks 返回背包里 potionID 的所有 (slot, count)，按 slot 升序。
func (s *slot) stacks(itemID int32) [][2]int32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out [][2]int32
	if s.bag == nil {
		return out
	}
	for _, it := range s.bag.Items {
		if it != nil && it.ItemId == itemID {
			out = append(out, [2]int32{it.Slot, it.Count})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
	return out
}

func fmtStacks(st [][2]int32) string {
	parts := make([]string, 0, len(st))
	for _, p := range st {
		parts = append(parts, fmt.Sprintf("slot%d=%d", p[0], p[1]))
	}
	if len(parts) == 0 {
		return "空"
	}
	return strings.Join(parts, " ")
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
			s := &slot{}
			slots[i] = s
			c := pomeloClient.New(pomeloClient.WithRequestTimeout(20 * time.Second))
			if err := c.ConnectToWS(addr, ""); err != nil {
				codes[i] = -1
				return
			}
			s.c = c
			c.On("onBagChange", func(msg *pomeloMessage.Message) {
				var bag protocol.BagListResponse
				if len(msg.Data) == 0 || c.Serializer().Unmarshal(msg.Data, &bag) != nil {
					return
				}
				s.mu.Lock()
				s.bag = &bag
				s.mu.Unlock()
			})
			var issued protocol.IssueTokenResponse
			code, err := call(c, "gate.user.issueToken", &protocol.IssueTokenRequest{
				Nickname: fmt.Sprintf("%s%d-%d", tag, stamp, i),
				Password: "123456",
				DeviceId: "lt11",
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
				AccessToken: issued.AccessToken, ServerId: 10001, DeviceId: "lt11",
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
