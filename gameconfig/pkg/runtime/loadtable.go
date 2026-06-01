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
// 一期支持: "item"=道具表。
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
		items := importdata.SchemaToItems(itemRows)
		newItem := cfg.NewTbItem(items)
		tables := cfg.NewTables(newItem)
		// 只替换 item 表，其他表保持不变
		swapSnapshot(&snapshot{
			version:    s.version,
			tableCount: int32(len(items)),
			tables:     tables,
		})
		return nil
	default:
		return fmt.Errorf("unknown config table: %s", tableName)
	}
}
