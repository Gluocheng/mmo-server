package runtime

import (
	"sync"

	"github.com/example/mmo-server/gameconfig/gen/cfg"
)

// tables 聚合 Luban 生成的反序列化行（按 id 索引）。
type tables struct {
	items    map[int32]*cfg.ItemItem
	bagTypes map[int32]*cfg.Bag_typeBagType
}

// snapshot 内存中的配置快照；Load 完成后整体替换。
type snapshot struct {
	version    int64
	tableCount int32
	tables     *tables
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
