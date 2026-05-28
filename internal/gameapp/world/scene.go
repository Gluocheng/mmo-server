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

func BroadcastMove(sender cfacade.IActor, fromUID int64, m *protocol.MoveBroadcast) {
	mu.RLock()
	from, ok := inRoom[fromUID]
	if !ok {
		mu.RUnlock()
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
	mu.RUnlock()

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
