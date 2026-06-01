package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/example/mmo-server/gameconfig/gen/cfg"
	"github.com/example/mmo-server/gameconfig/pkg/importdata"
	"github.com/example/mmo-server/gameconfig/pkg/schema"
	"gorm.io/gorm"
)

var (
	// ErrNotLoaded 配置尚未 Load。
	ErrNotLoaded = errors.New("gameconfig not loaded")
	// ErrItemNotFound 道具 id 不在配置表。
	ErrItemNotFound = errors.New("item not found in config")
)

// Load 从 MySQL 读取配置表并构建内存快照。
func Load(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("gameconfig load: db is nil")
	}

	var versionRow schema.CfgVersion
	err := db.WithContext(ctx).First(&versionRow, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		versionRow = schema.CfgVersion{ID: 1, Version: 0}
	} else if err != nil {
		return fmt.Errorf("load cfg_version: %w", err)
	}

	var itemRows []schema.CfgItem
	if err := db.WithContext(ctx).Order("id asc").Find(&itemRows).Error; err != nil {
		return fmt.Errorf("load cfg_item: %w", err)
	}

	items := importdata.SchemaToItems(itemRows)
	tables := cfg.NewTables(cfg.NewTbItem(items))
	tableCount := int32(0)
	if tables.TbItem != nil {
		tableCount = int32(len(tables.TbItem.DataList()))
	}

	swapSnapshot(&snapshot{
		version:    versionRow.Version,
		tableCount: tableCount,
		tables:     tables,
	})
	return nil
}

// MustLoad 加载失败则 panic（game 节点启动用）。
func MustLoad(ctx context.Context, db *gorm.DB) {
	if err := Load(ctx, db); err != nil {
		panic(fmt.Sprintf("gameconfig MustLoad: %v", err))
	}
}

// Version 返回当前内存中的配置版本号。
func Version() int64 {
	s := getSnapshot()
	if s == nil {
		return 0
	}
	return s.version
}

// TableCount 返回已加载的配置表行数（首期仅 item 表行数）。
func TableCount() int32 {
	s := getSnapshot()
	if s == nil {
		return 0
	}
	return s.tableCount
}

// Tables 返回底层 Tables（只读使用，勿修改）。
func Tables() (*cfg.Tables, error) {
	s := getSnapshot()
	if s == nil || s.tables == nil {
		return nil, ErrNotLoaded
	}
	return s.tables, nil
}

// BuildFromItems 直接从 item 行构建快照（单测用，不写 DB）。
func BuildFromItems(items []*cfg.Item, version int64) {
	tables := cfg.NewTables(cfg.NewTbItem(items))
	swapSnapshot(&snapshot{
		version:    version,
		tableCount: int32(len(items)),
		tables:     tables,
	})
}
