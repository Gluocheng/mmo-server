package persistence

import (
	"sync"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// UseMemoryDBForTest 将全局 DB 换成内存 SQLite，并清空内存会话。仅测试调用。
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

	oldDB := db
	oldRDB := rdb
	oldInitErr := initErr
	hadInit := oldDB != nil
	db = gdb
	rdb = nil
	once = sync.Once{}
	initErr = nil

	gmSessMu.Lock()
	gmSessMem = map[string]memSess{}
	gmSessMu.Unlock()

	t.Cleanup(func() {
		db = oldDB
		rdb = oldRDB
		initErr = oldInitErr
		once = sync.Once{}
		if hadInit {
			once.Do(func() {})
		}
		gmSessMu.Lock()
		gmSessMem = map[string]memSess{}
		gmSessMu.Unlock()
	})
	return gdb
}
