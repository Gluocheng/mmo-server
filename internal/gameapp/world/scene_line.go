package world

import (
	"sort"

	cfacade "github.com/cherry-game/cherry/facade"
	"github.com/cherry-game/cherry/net/parser/pomelo"
	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/protocol"
)

// LineRule 是一张地图的进房规则，数值来自配表。
type LineRule struct {
	SceneID    int32
	MaxOnline  int32
	MaxLines   int32
	SpawnX     float32
	SpawnY     float32
	SpawnZ     float32
	SwitchCdMs int32
}

// Nearby 是同一条线 AOI 内的其他单位。ActorType 为 0 表示玩家。
type Nearby struct {
	UID       int64
	X, Y, Z   float32
	ActorType int32
	ConfigID  int32
}

// RoomView 是进场或切图后回给客户端的位置。Changed 为 true 时调用方要重置战斗。
type RoomView struct {
	SceneID int32
	Line    int32
	X, Y, Z float32
	Nearby  []Nearby
	Changed bool
}

type presencePeer struct {
	UID       int64
	AgentPath string
}

// ArriveMain 进入主城。已在房间里的同 uid 会先离开旧线。失败时不占坑。
func ArriveMain(sender cfacade.IActor, uid int64, agentPath string, rule LineRule) (RoomView, int32) {
	if uid < 1 || rule.SceneID < 1 || rule.MaxOnline < 1 {
		return RoomView{}, code.SceneInvalid
	}
	mu.Lock()
	var left *protocol.ScenePresence
	var leftPeers []presencePeer
	if old, ok := inRoom[uid]; ok {
		leftPeers = peersOf(old, uid)
		left = presenceOf(uid, old, false)
		delete(inRoom, uid)
	}
	line, ok := pickLine(rule, uid)
	if !ok {
		if left != nil {
			inRoom[uid] = playerState{
				agentPath: agentPath, sceneID: left.SceneId, line: left.Line,
				x: left.X, y: left.Y, z: left.Z,
			}
		}
		mu.Unlock()
		return RoomView{}, code.SceneFull
	}
	st := playerState{
		agentPath: agentPath, sceneID: rule.SceneID, line: line,
		x: rule.SpawnX, y: rule.SpawnY, z: rule.SpawnZ,
	}
	inRoom[uid] = st
	view := viewOf(uid, st)
	enterPeers := peersOf(st, uid)
	mu.Unlock()

	if left != nil {
		pushPresence(sender, leftPeers, left)
	}
	pushPresence(sender, enterPeers, presenceOf(uid, st, true))
	return view, code.OK
}

// SwitchMap 换到目标图。目标就是当前图时留在当前线，不重置冷却。
// leftCdMs 是离开当前图应开始的冷却，只在真正换图成功时写入。
func SwitchMap(sender cfacade.IActor, uid int64, target LineRule, leftCdMs int32, now int64) (RoomView, int32) {
	if uid < 1 {
		return RoomView{}, code.PlayerNotEntered
	}
	mu.Lock()
	cur, ok := inRoom[uid]
	if !ok {
		mu.Unlock()
		return RoomView{}, code.PlayerNotEntered
	}
	if target.SceneID == cur.sceneID {
		view := viewOf(uid, cur)
		mu.Unlock()
		return view, code.OK
	}
	if target.SceneID < 1 || target.MaxOnline < 1 {
		mu.Unlock()
		return RoomView{}, code.SceneInvalid
	}
	if cur.nextSwitchAt > 0 && now < cur.nextSwitchAt {
		mu.Unlock()
		return RoomView{}, code.SceneSwitchCooldown
	}
	line, picked := pickLine(target, uid)
	if !picked {
		mu.Unlock()
		return RoomView{}, code.SceneFull
	}
	leftPeers := peersOf(cur, uid)
	leftMsg := presenceOf(uid, cur, false)
	next := now
	if leftCdMs > 0 {
		next = now + int64(leftCdMs)
	}
	st := playerState{
		agentPath: cur.agentPath, sceneID: target.SceneID, line: line,
		x: target.SpawnX, y: target.SpawnY, z: target.SpawnZ,
		nextSwitchAt: next,
	}
	inRoom[uid] = st
	view := viewOf(uid, st)
	view.Changed = true
	enterPeers := peersOf(st, uid)
	mu.Unlock()

	pushPresence(sender, leftPeers, leftMsg)
	pushPresence(sender, enterPeers, presenceOf(uid, st, true))
	return view, code.OK
}

// LineOnline 返回某条线上的人数，不含未进场。
func LineOnline(sceneID, line int32) int32 {
	mu.RLock()
	defer mu.RUnlock()
	return countLine(sceneID, line, 0)
}

func pickLine(rule LineRule, skip int64) (int32, bool) {
	n := rule.MaxLines
	if n < 2 {
		n = 1
	}
	for line := int32(1); line <= n; line++ {
		if countLine(rule.SceneID, line, skip) < rule.MaxOnline {
			return line, true
		}
	}
	return 0, false
}

func countLine(sceneID, line int32, skip int64) int32 {
	var n int32
	for uid, st := range inRoom {
		if uid == skip || st.actorType == ActorMonster || st.sceneID != sceneID || st.line != line {
			continue
		}
		n++
	}
	return n
}

func viewOf(self int64, st playerState) RoomView {
	return RoomView{
		SceneID: st.sceneID, Line: st.line,
		X: st.x, Y: st.y, Z: st.z,
		Nearby: nearbyOf(st, self),
	}
}

func nearbyOf(st playerState, skip int64) []Nearby {
	out := make([]Nearby, 0)
	for uid, other := range inRoom {
		if uid == skip || other.sceneID != st.sceneID || other.line != st.line {
			continue
		}
		if !withinAOI(st.x, st.z, other.x, other.z) {
			continue
		}
		out = append(out, Nearby{
			UID: uid, X: other.x, Y: other.y, Z: other.z,
			ActorType: other.actorType, ConfigID: other.configID,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UID < out[j].UID })
	return out
}

func peersOf(st playerState, self int64) []presencePeer {
	near := nearbyOf(st, self)
	peers := make([]presencePeer, 0, len(near))
	for _, n := range near {
		other := inRoom[n.UID]
		peers = append(peers, presencePeer{UID: n.UID, AgentPath: other.agentPath})
	}
	return peers
}

func presenceOf(uid int64, st playerState, enter bool) *protocol.ScenePresence {
	return &protocol.ScenePresence{
		Uid: uid, SceneId: st.sceneID, Line: st.line,
		X: st.x, Y: st.y, Z: st.z, Enter: enter,
		ActorType: st.actorType, ConfigId: st.configID,
	}
}

// PlaceMonster 把怪物放到指定线的坐标上。它不占人数上限。
func PlaceMonster(uid int64, sceneID, line int32, x, y, z float32, configID int32) {
	if uid < 1 || sceneID < 1 {
		return
	}
	if line < 1 {
		line = 1
	}
	mu.Lock()
	defer mu.Unlock()
	inRoom[uid] = playerState{
		sceneID: sceneID, line: line, x: x, y: y, z: z,
		actorType: ActorMonster, configID: configID,
	}
}

// AnnounceMonster 通知同线 AOI 内的玩家：这只怪物出现了。调用前要先 PlaceMonster。
func AnnounceMonster(sender cfacade.IActor, uid int64) {
	mu.RLock()
	st, ok := inRoom[uid]
	if !ok {
		mu.RUnlock()
		return
	}
	peers := peersOf(st, uid)
	msg := presenceOf(uid, st, true)
	mu.RUnlock()
	pushPresence(sender, peers, msg)
}

// DropMonster 从房间移除怪物，并通知周围玩家它离开了。
func DropMonster(sender cfacade.IActor, uid int64) {
	mu.Lock()
	st, ok := inRoom[uid]
	if !ok {
		mu.Unlock()
		return
	}
	peers := peersOf(st, uid)
	msg := presenceOf(uid, st, false)
	delete(inRoom, uid)
	mu.Unlock()
	pushPresence(sender, peers, msg)
}

func pushPresence(sender cfacade.IActor, peers []presencePeer, msg *protocol.ScenePresence) {
	if sender == nil || msg == nil {
		return
	}
	for _, peer := range peers {
		if peer.AgentPath == "" || peer.UID == msg.Uid {
			continue
		}
		pomelo.PushWithUID(sender, peer.AgentPath, peer.UID, "onScenePresence", msg)
	}
}
