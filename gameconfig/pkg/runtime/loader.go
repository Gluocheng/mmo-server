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

	var bagTypeRows []schema.CfgBagType
	if err := db.WithContext(ctx).Order("id asc").Find(&bagTypeRows).Error; err != nil {
		return fmt.Errorf("load cfg_bag_type: %w", err)
	}

	items := make(map[int32]*cfg.ItemItem, len(itemRows))
	for i := range itemRows {
		it := importdata.SchemaToItems(itemRows)[i]
		items[it.Id] = it
	}
	bagTypes := make(map[int32]*cfg.Bag_typeBagType, len(bagTypeRows))
	for i := range bagTypeRows {
		bt := importdata.SchemaToBagTypes(bagTypeRows)[i]
		bagTypes[bt.Id] = bt
	}

	swapSnapshot(&snapshot{
		version:    versionRow.Version,
		tableCount: int32(len(itemRows)),
		tables: &tables{
			items:    items,
			bagTypes: bagTypes,
		},
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

// BuildFromItems 直接从 item 行构建快照（单测用，不写 DB）。
func BuildFromItems(items []*cfg.ItemItem, version int64) {
	m := make(map[int32]*cfg.ItemItem, len(items))
	for _, it := range items {
		if it == nil {
			continue
		}
		m[it.Id] = it
	}
	swapSnapshot(&snapshot{
		version:    version,
		tableCount: int32(len(items)),
		tables: &tables{
			items:    m,
			bagTypes: nil,
		},
	})
}
