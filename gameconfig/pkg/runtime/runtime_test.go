package runtime_test

import (
	"context"
	"testing"

	"github.com/example/mmo-server/gameconfig/gen/cfg"
	"github.com/example/mmo-server/gameconfig/pkg/runtime"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gdb, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return gdb
}

func TestLoadAndReload(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := runtime.SeedDemoItems(ctx, db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if !runtime.Exists(1001) {
		t.Fatal("expected item 1001")
	}
	if runtime.MaxStack(1001) != 99 {
		t.Fatalf("max stack want 99 got %d", runtime.MaxStack(1001))
	}
	if err := runtime.Reload(ctx, db); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if runtime.Version() < 1 {
		t.Fatalf("version want >=1 got %d", runtime.Version())
	}
}

func TestBuildFromItemsConcurrentRead(t *testing.T) {
	items := []*cfg.Item{{ID: 7, Name: "x", Type: "material", MaxStack: 10, Stackable: true, Discardable: true, BindType: "none"}}
	runtime.BuildFromItems(items, 1)
	if !runtime.Exists(7) {
		t.Fatal("expected item 7")
	}
	if runtime.MaxStack(7) != 10 {
		t.Fatalf("got %d", runtime.MaxStack(7))
	}
}
