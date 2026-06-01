package runtime

import (
	"sync"

	"github.com/example/mmo-server/gameconfig/gen/cfg"
)

// snapshot 内存中的配置快照；Load 完成后整体替换。
type snapshot struct {
	version    int64
	tableCount int32
	tables     *cfg.Tables
}

var (
	mu       sync.RWMutex
	current  *snapshot
	loadedDB bool
)

func getSnapshot() *snapshot {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

func swapSnapshot(next *snapshot) {
	mu.Lock()
	current = next
	loadedDB = true
	mu.Unlock()
}

func isLoaded() bool {
	mu.RLock()
	defer mu.RUnlock()
	return loadedDB && current != nil
}
