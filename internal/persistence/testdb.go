package persistence

import (
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// UseMemoryDBForTest 将玩家库和配置库都换成同一个内存 SQLite，并清空内存会话。仅测试调用。
func UseMemoryDBForTest(t *testing.T) *gorm.DB {
	t.Helper()
	gdb, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := autoMigrateModels(gdb); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := autoMigrateConfig(gdb); err != nil {
		t.Fatalf("migrate config: %v", err)
	}

	resetPlayerIDBatch()

	oldDB := db
	oldRDB := rdb
	oldInitErr := initErr
	oldCfgDB := cfgDB
	oldCfgErr := cfgInitErr
	hadInit := oldDB != nil
	db = gdb
	cfgDB = gdb
	rdb = nil
	once = sync.Once{}
	cfgOnce = sync.Once{}
	initErr = nil
	cfgInitErr = nil

	gmSessMu.Lock()
	gmSessMem = map[string]memSess{}
	gmSessMu.Unlock()

	t.Cleanup(func() {
		db = oldDB
		cfgDB = oldCfgDB
		rdb = oldRDB
		initErr = oldInitErr
		cfgInitErr = oldCfgErr
		once = sync.Once{}
		cfgOnce = sync.Once{}
		if hadInit {
			once.Do(func() {})
		}
		gmSessMu.Lock()
		gmSessMem = map[string]memSess{}
		gmSessMu.Unlock()
	})
	return gdb
}

// UseRedisForTest 挂接 miniredis 作为全局 rdb；须在 UseMemoryDBForTest 之后调用。
func UseRedisForTest(t *testing.T) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	oldRDB := rdb
	oldCfg := cfg
	rdb = client
	if cfg.keyPrefix == "" {
		cfg.keyPrefix = "t"
	}
	if cfg.accessTTL == 0 {
		cfg.accessTTL = time.Minute
	}
	if cfg.refreshTTL == 0 {
		cfg.refreshTTL = time.Hour
	}
	t.Cleanup(func() {
		_ = client.Close()
		rdb = oldRDB
		cfg = oldCfg
	})
}
