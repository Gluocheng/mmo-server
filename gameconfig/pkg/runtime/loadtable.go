package runtime

import (
	"context"
	"fmt"

	"github.com/example/mmo-server/gameconfig/gen/cfg"
	"github.com/example/mmo-server/gameconfig/pkg/importdata"
	"github.com/example/mmo-server/gameconfig/pkg/schema"
	"gorm.io/gorm"
)

// ReloadTable 按表名重新加载指定配置表；失败时保留旧快照。
// 支持: item、bag_type、skill、buff、combat_const。后三张整包重载，失败保留旧快照。
func ReloadTable(ctx context.Context, db *gorm.DB, tableName string) error {
	if db == nil {
		return fmt.Errorf("gameconfig reload table: db is nil")
	}
	s := getSnapshot()
	if s == nil || s.tables == nil {
		return ErrNotLoaded
	}

	switch tableName {
	case "item":
		var itemRows []schema.CfgItem
		if err := db.WithContext(ctx).Order("id asc").Find(&itemRows).Error; err != nil {
			return fmt.Errorf("reload cfg_item: %w", err)
		}
		items := make(map[int32]*cfg.ItemItem, len(itemRows))
		for i := range itemRows {
			it := importdata.SchemaToItems(itemRows)[i]
			items[it.Id] = it
		}
		// 只替换 item 表，其他表保持不变
		next := s.tables.clone()
		next.items = items
		swapSnapshot(&snapshot{
			version:    s.version,
			tableCount: int32(len(itemRows)),
			tables:     next,
		})
		return nil
	case "bag_type":
		var bagTypeRows []schema.CfgBagType
		if err := db.WithContext(ctx).Order("id asc").Find(&bagTypeRows).Error; err != nil {
			return fmt.Errorf("reload cfg_bag_type: %w", err)
		}
		bagTypes := make(map[int32]*cfg.Bag_typeBagType, len(bagTypeRows))
		for i := range bagTypeRows {
			bt := importdata.SchemaToBagTypes(bagTypeRows)[i]
			bagTypes[bt.Id] = bt
		}
		// 只替换 bag_type 表，其他表保持不变
		next := s.tables.clone()
		next.bagTypes = bagTypes
		swapSnapshot(&snapshot{
			version:    s.version,
			tableCount: s.tableCount,
			tables:     next,
		})
		return nil
	case "skill", "buff", "combat_const":
		return Reload(ctx, db)
	default:
		return fmt.Errorf("unknown config table: %s", tableName)
	}
}
