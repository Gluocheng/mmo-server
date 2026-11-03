package broadcast

import (
	"sync"

	cfacade "github.com/cherry-game/cherry/facade"
	"github.com/cherry-game/cherry/net/parser/pomelo"
	"github.com/example/mmo-server/internal/gameapp/combat"
	"github.com/example/mmo-server/internal/gameapp/world"
	"github.com/example/mmo-server/internal/protocol"
)

// QueueCap 移动和聊天共用的广播队列长度。
const QueueCap = 4096

// ErrFull 聊天队列已满。移动满了只丢掉这一档广播，不返回这个错误。
var ErrFull = errFull{}

type errFull struct{}

func (errFull) Error() string { return "broadcast queue full" }

type jobKind int

const (
	jobMove jobKind = iota
	jobChat
)

type job struct {
	kind    jobKind
	uid     int64
	gen     uint64
	sceneID int32
	text    string
}

type seen struct {
	gen uint64
	ids []int64
}

var (
	mu      sync.Mutex
	cond    = sync.NewCond(&mu)
	queue   []job
	waiting = map[int64]int{}
	depth   int
	seenOf  = map[int64]seen{}
	sender  cfacade.IActor
	started bool
	capN    = QueueCap
)

// Bind 记下用来推送的 Actor，并启动一条广播循环。重复调用只保留第一次的 Actor。
func Bind(actor cfacade.IActor) {
	mu.Lock()
	if sender == nil {
		sender = actor
	}
	if !started {
		started = true
		go loop()
	}
	mu.Unlock()
	cond.Signal()
}

// NoteSeen 记下这个玩家当前视野，作为下一次移动进出视野的基准。进场和切图成功后调用。
func NoteSeen(uid int64) {
	ids := append([]int64(nil), world.NearbyUIDs(uid)...)
	mu.Lock()
	cur := seenOf[uid]
	cur.gen++
	if cur.gen == 0 {
		cur.gen = 1
	}
	cur.ids = ids
	seenOf[uid] = cur
	mu.Unlock()
}

// Forget 丢掉玩家的视野基准和还没推的移动。断线时调用。
func Forget(uid int64) {
	mu.Lock()
	delete(seenOf, uid)
	dropMoveLocked(uid)
	mu.Unlock()
}

// EnqueueMove 把一次移动放进广播队列。同一人还有没推完的移动时只保留队列里的那一格，坐标以推送时的场景为准。
// 队列满且这个人还没在队里时丢掉这次广播，调用方仍然可以先回包。
func EnqueueMove(uid int64) bool {
	mu.Lock()
	defer mu.Unlock()
	if _, ok := waiting[uid]; ok {
		cond.Signal()
		return true
	}
	if depth >= capN {
		return false
	}
	gen := uint64(0)
	if cur, ok := seenOf[uid]; ok {
		gen = cur.gen
	}
	waiting[uid] = len(queue)
	queue = append(queue, job{kind: jobMove, uid: uid, gen: gen})
	depth++
	cond.Signal()
	return true
}

// EnqueueChat 尝试占一个广播名额。满了返回 ErrFull，调用方不要回成功。
func EnqueueChat(uid int64, sceneID int32, text string) error {
	mu.Lock()
	defer mu.Unlock()
	if depth >= capN {
		return ErrFull
	}
	queue = append(queue, job{kind: jobChat, uid: uid, sceneID: sceneID, text: text})
	depth++
	cond.Signal()
	return nil
}

func loop() {
	for {
		job, ok := pop()
		if !ok {
			continue
		}
		switch job.kind {
		case jobMove:
			flushMove(job)
		case jobChat:
			flushChat(job)
		}
	}
}

func pop() (job, bool) {
	mu.Lock()
	defer mu.Unlock()
	for len(queue) == 0 {
		cond.Wait()
	}
	job := queue[0]
	queue = queue[1:]
	depth--
	reindexWaitingLocked()
	return job, true
}

func dropMoveLocked(uid int64) {
	idx, ok := waiting[uid]
	if !ok || idx < 0 || idx >= len(queue) {
		delete(waiting, uid)
		return
	}
	queue = append(queue[:idx], queue[idx+1:]...)
	depth--
	reindexWaitingLocked()
}

func reindexWaitingLocked() {
	waiting = map[int64]int{}
	for i, job := range queue {
		if job.kind == jobMove {
			waiting[job.uid] = i
		}
	}
}

func flushMove(job job) {
	mu.Lock()
	cur, ok := seenOf[job.uid]
	actor := sender
	mu.Unlock()
	if !ok || cur.gen != job.gen {
		return
	}
	pose, in := world.PoseOf(job.uid)
	if !in {
		Forget(job.uid)
		return
	}
	if actor != nil {
		msg := &protocol.MoveBroadcast{Uid: job.uid, X: pose.X, Y: pose.Y, Z: pose.Z}
		for _, id := range world.NearbyUIDs(job.uid) {
			path, has := world.AgentPath(id)
			if !has || path == "" {
				continue
			}
			pomelo.PushWithUID(actor, path, id, "onMove", msg)
		}
		combat.PublishMove(actor, job.uid, cur.ids)
	}
	remember(job.uid, world.NearbyUIDs(job.uid))
}

// remember 更新已广播视野，不推进代际。代际只在进场、切图和断线时变化。
func remember(uid int64, ids []int64) {
	mu.Lock()
	cur, ok := seenOf[uid]
	if !ok {
		mu.Unlock()
		return
	}
	cur.ids = append([]int64(nil), ids...)
	seenOf[uid] = cur
	mu.Unlock()
}

func flushChat(job job) {
	mu.Lock()
	actor := sender
	mu.Unlock()
	if actor == nil {
		return
	}
	world.BroadcastChat(actor, job.uid, job.sceneID, &protocol.ChatBroadcast{Uid: job.uid, Text: job.text})
}
