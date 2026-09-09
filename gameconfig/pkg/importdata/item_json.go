package importdata

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/example/mmo-server/gameconfig/gen/cfg"
	"github.com/example/mmo-server/gameconfig/pkg/schema"
)

const ItemTableFile = "item_tbitem.json"

// LoadItemsFromJSONFile 读取 Luban 导出的道具 JSON 数组并反序列化为表。
// 返回表与原始行切片；行切片用于写库（schema）。
func LoadItemsFromJSONFile(path string) (*cfg.ItemTbItem, []map[string]interface{}, error) {
	rows, err := loadJSONRows(path)
	if err != nil {
		return nil, nil, err
	}
	for _, row := range rows {
		id, _ := row["id"].(float64)
		if id < 1 {
			return nil, nil, fmt.Errorf("invalid item row in %s", path)
		}
	}
	table, err := cfg.NewItemTbItem(rows)
	if err != nil {
		return nil, nil, fmt.Errorf("build item table: %w", err)
	}
	return table, rows, nil
}

// ItemsToSchema 将 Luban 反序列化出的 ItemItem 列表转为 GORM 行。
func ItemsToSchema(items []*cfg.ItemItem) []schema.CfgItem {
	out := make([]schema.CfgItem, 0, len(items))
	for _, it := range items {
		if it == nil {
			continue
		}
		out = append(out, schema.CfgItem{
			ID:          it.Id,
			Name:        it.Name,
			Type:        it.Type,
			MaxStack:    it.MaxStack,
			Stackable:   it.Stackable,
			Discardable: it.Discardable,
			BindType:    it.BindType,
			BagType:     it.BagType,
		})
	}
	return out
}

// SchemaToItems 将 DB 行转为 cfg.ItemItem（供 Load 构建内存表）。
func SchemaToItems(rows []schema.CfgItem) []*cfg.ItemItem {
	out := make([]*cfg.ItemItem, 0, len(rows))
	for i := range rows {
		r := rows[i]
		out = append(out, &cfg.ItemItem{
			Id:          r.ID,
			Name:        r.Name,
			Type:        r.Type,
			MaxStack:    r.MaxStack,
			Stackable:   r.Stackable,
			Discardable: r.Discardable,
			BindType:    r.BindType,
			BagType:     r.BagType,
		})
	}
	return out
}

// loadJSONRows 读取 Luban 导出的 JSON 数组为 []map[string]interface{}。
func loadJSONRows(path string) ([]map[string]interface{}, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rows []map[string]interface{}
	if err := json.Unmarshal(b, &rows); err != nil {
		return nil, fmt.Errorf("parse json %s: %w", path, err)
	}
	return rows, nil
}
