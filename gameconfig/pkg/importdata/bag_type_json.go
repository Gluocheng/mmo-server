package importdata

import (
	"fmt"

	"github.com/example/mmo-server/gameconfig/gen/cfg"
	"github.com/example/mmo-server/gameconfig/pkg/schema"
)

// BagTypeTableFile 是 Luban 导出的背包类型 JSON 文件名。
const BagTypeTableFile = "bag_type_tbbagtype.json"

// LoadBagTypesFromJSONFile 读取 Luban 导出的背包类型 JSON 数组并反序列化为表。
func LoadBagTypesFromJSONFile(path string) (*cfg.Bag_typeTbBagType, []map[string]interface{}, error) {
	rows, err := loadJSONRows(path)
	if err != nil {
		return nil, nil, err
	}
	for _, row := range rows {
		id, _ := row["id"].(float64)
		if id < 1 {
			return nil, nil, fmt.Errorf("invalid bag_type row in %s", path)
		}
	}
	table, err := cfg.NewBag_typeTbBagType(rows)
	if err != nil {
		return nil, nil, fmt.Errorf("build bag_type table: %w", err)
	}
	return table, rows, nil
}

// BagTypesToSchema 将 Luban Bag_typeBagType 列表转为 GORM 行。
func BagTypesToSchema(items []*cfg.Bag_typeBagType) []schema.CfgBagType {
	out := make([]schema.CfgBagType, 0, len(items))
	for _, it := range items {
		if it == nil {
			continue
		}
		out = append(out, schema.CfgBagType{
			ID:        it.Id,
			Name:      it.Name,
			SlotCount: it.SlotCount,
		})
	}
	return out
}

// SchemaToBagTypes 将 DB 行转为 cfg.Bag_typeBagType（供 Load 构建内存表）。
func SchemaToBagTypes(rows []schema.CfgBagType) []*cfg.Bag_typeBagType {
	out := make([]*cfg.Bag_typeBagType, 0, len(rows))
	for i := range rows {
		r := rows[i]
		out = append(out, &cfg.Bag_typeBagType{
			Id:        r.ID,
			Name:      r.Name,
			SlotCount: r.SlotCount,
		})
	}
	return out
}
