package importdata

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/example/mmo-server/gameconfig/gen/cfg"
	"github.com/example/mmo-server/gameconfig/pkg/schema"
)

const ItemTableFile = "item_tbitem.json"

// LoadItemsFromJSONFile 读取 Luban 导出的道具 JSON 数组。
func LoadItemsFromJSONFile(path string) ([]*cfg.Item, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rows []*cfg.Item
	if err := json.Unmarshal(b, &rows); err != nil {
		return nil, fmt.Errorf("parse item json: %w", err)
	}
	for _, row := range rows {
		if row == nil || row.ID < 1 {
			return nil, fmt.Errorf("invalid item row in %s", path)
		}
		if !row.Stackable && row.MaxStack != 1 {
			row.MaxStack = 1
		}
		if row.MaxStack < 1 {
			return nil, fmt.Errorf("item %d max_stack invalid", row.ID)
		}
		if row.BindType == "" {
			row.BindType = "none"
		}
	}
	return rows, nil
}

// ItemsToSchema 将 cfg.Item 转为 GORM 行。
func ItemsToSchema(rows []*cfg.Item) []schema.CfgItem {
	out := make([]schema.CfgItem, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		out = append(out, schema.CfgItem{
			ID:          row.ID,
			Name:        row.Name,
			Type:        row.Type,
			MaxStack:    row.MaxStack,
			Stackable:   row.Stackable,
			Discardable: row.Discardable,
			BindType:    row.BindType,
		})
	}
	return out
}

// SchemaToItems 将 DB 行转为 cfg.Item。
func SchemaToItems(rows []schema.CfgItem) []*cfg.Item {
	out := make([]*cfg.Item, 0, len(rows))
	for i := range rows {
		r := rows[i]
		out = append(out, &cfg.Item{
			ID:          r.ID,
			Name:        r.Name,
			Type:        r.Type,
			MaxStack:    r.MaxStack,
			Stackable:   r.Stackable,
			Discardable: r.Discardable,
			BindType:    r.BindType,
		})
	}
	return out
}
