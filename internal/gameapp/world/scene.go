package world

import (
	"math"
	"sync"

	cfacade "github.com/cherry-game/cherry/facade"
	clog "github.com/cherry-game/cherry/logger"
	"github.com/cherry-game/cherry/net/parser/pomelo"
	"github.com/example/mmo-server/internal/protocol"
)

// DefaultSceneID 演示用固定场景
const DefaultSceneID = int32(1)

const (
	aoiRadius = float32(15) // 演示用 AOI 半径
)

type playerState struct {
	agentPath string
	sceneID   int32
	x, y, z   float32
}

var (
	mu     sync.RWMutex
	inRoom = make(map[int64]playerState) // uid -> state
)

func Enter(uid int64, agentPath string, sceneID int32) []int64 {
	mu.Lock()
	defer mu.Unlock()
	inRoom[uid] = playerState{
		agentPath: agentPath,
		sceneID:   sceneID,
	}
	out := make([]int64, 0, len(inRoom))
	for u, st := range inRoom {
		if st.sceneID != sceneID {
			continue
		}
		out = append(out, u)
	}
	return out
}

func Leave(uid int64) {
	mu.Lock()
	defer mu.Unlock()
	delete(inRoom, uid)
}

func SceneID(uid int64) (int32, bool) {
	mu.RLock()
	defer mu.RUnlock()
	st, ok := inRoom[uid]
	if !ok {
		return 0, false
	}
	return st.sceneID, true
}

// AgentPath 返回已进场玩家的网关 agent 路径；未进场 ok=false。
func AgentPath(uid int64) (string, bool) {
	mu.RLock()
	defer mu.RUnlock()
	st, ok := inRoom[uid]
	if !ok || st.agentPath == "" {
		return "", false
	}
	return st.agentPath, true
}

func BroadcastMove(sender cfacade.IActor, fromUID int64, m *protocol.MoveBroadcast) {
	if m == nil {
		return
	}
	mu.Lock()
	from, ok := inRoom[fromUID]
	if !ok {
		mu.Unlock()
		return
	}
	from.x, from.y, from.z = m.X, m.Y, m.Z
	inRoom[fromUID] = from

	peers := make(map[int64]string)
	for u, st := range inRoom {
		if u == fromUID {
			continue
		}
		if st.sceneID != from.sceneID {
			continue
		}
		if !withinAOI(from.x, from.z, st.x, st.z) {
			continue
		}
		peers[u] = st.agentPath
	}
	mu.Unlock()

	for uid, path := range peers {
		pomelo.PushWithUID(sender, path, uid, "onMove", m)
	}
	clog.Debugf("broadcast move to %d peers", len(peers))
}

func withinAOI(x1, z1, x2, z2 float32) bool {
	dx := float64(x1 - x2)
	dz := float64(z1 - z2)
	return math.Sqrt(dx*dx+dz*dz) <= float64(aoiRadius)
}

// InAOI 判断两点是否落在演示用 AOI 半径内。
func InAOI(x1, z1, x2, z2 float32) bool {
	return withinAOI(x1, z1, x2, z2)
}

// Pose 是已进场玩家的场景坐标。
type Pose struct {
	UID       int64
	SceneID   int32
	X, Z      float32
	AgentPath string
}

// PoseOf 返回已进场玩家坐标。
func PoseOf(uid int64) (Pose, bool) {
	mu.RLock()
	defer mu.RUnlock()
	st, ok := inRoom[uid]
	if !ok {
		return Pose{}, false
	}
	return Pose{UID: uid, SceneID: st.sceneID, X: st.x, Z: st.z, AgentPath: st.agentPath}, true
}

// PosesInScene 返回该场景内全部已进场玩家。
func PosesInScene(sceneID int32) []Pose {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Pose, 0)
	for uid, st := range inRoom {
		if st.sceneID != sceneID {
			continue
		}
		out = append(out, Pose{UID: uid, SceneID: st.sceneID, X: st.x, Z: st.z, AgentPath: st.agentPath})
	}
	return out
}

// SetPosition 更新已进场玩家坐标。未进场返回 false。
func SetPosition(uid int64, x, y, z float32) bool {
	mu.Lock()
	defer mu.Unlock()
	st, ok := inRoom[uid]
	if !ok {
		return false
	}
	st.x, st.y, st.z = x, y, z
	inRoom[uid] = st
	return true
}

func BroadcastChat(sender cfacade.IActor, fromUID int64, sceneID int32, m *protocol.ChatBroadcast) {
	mu.RLock()
	peers := make(map[int64]string)
	for u, st := range inRoom {
		if u == fromUID {
			continue
		}
		if st.sceneID != sceneID {
			continue
		}
		peers[u] = st.agentPath
	}
	mu.RUnlock()

	for uid, path := range peers {
		pomelo.PushWithUID(sender, path, uid, "onChat", m)
	}
}

// OnlinePlayer 已进场玩家快照。
type OnlinePlayer struct {
	UID     int64
	SceneID int32
}

// ListOnline 返回已进场玩家；sceneID=0 为全部场景。
func ListOnline(sceneID int32) []OnlinePlayer {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]OnlinePlayer, 0, len(inRoom))
	for uid, st := range inRoom {
		if sceneID != 0 && st.sceneID != sceneID {
			continue
		}
		out = append(out, OnlinePlayer{UID: uid, SceneID: st.sceneID})
	}
	return out
}

// BroadcastNotice 向场景内全部在线玩家推送公告（不跳过任何人）；sceneID=0 为全服。返回推送人数。
func BroadcastNotice(sender cfacade.IActor, sceneID int32, m *protocol.GmNoticePush) int32 {
	if m == nil {
		return 0
	}
	mu.RLock()
	peers := make(map[int64]string)
	for u, st := range inRoom {
		if sceneID != 0 && st.sceneID != sceneID {
			continue
		}
		if st.agentPath == "" {
			continue
		}
		peers[u] = st.agentPath
	}
	mu.RUnlock()

	for uid, path := range peers {
		pomelo.PushWithUID(sender, path, uid, "onNotice", m)
	}
	return int32(len(peers))
}
