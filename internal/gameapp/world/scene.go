package world

import (
	"sync"

	cfacade "github.com/cherry-game/cherry/facade"
	clog "github.com/cherry-game/cherry/logger"
	"github.com/cherry-game/cherry/net/parser/pomelo"
	"github.com/example/mmo-server/internal/protocol"
)

// DefaultSceneID 演示用固定场景
const DefaultSceneID = int32(1)

var (
	mu     sync.RWMutex
	inRoom = make(map[int64]string) // uid -> agentPath
)

func Enter(uid int64, agentPath string) []int64 {
	mu.Lock()
	defer mu.Unlock()
	inRoom[uid] = agentPath
	out := make([]int64, 0, len(inRoom))
	for u := range inRoom {
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
	peers := make(map[int64]string, len(inRoom))
	for u, path := range inRoom {
		if u != fromUID {
			peers[u] = path
		}
	}
	mu.RUnlock()

	for uid, path := range peers {
		pomelo.PushWithUID(sender, path, uid, "onMove", m)
	}
	clog.Debugf("broadcast move to %d peers", len(peers))
}
