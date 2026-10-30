package importdata

import (
	"fmt"

	"github.com/example/mmo-server/gameconfig/pkg/schema"
)

const (
	// EquipSlotTableFile 是部位 JSON。
	EquipSlotTableFile = "equip_slot.json"
	// EquipAttrTableFile 是装备属性 JSON。
	EquipAttrTableFile = "equip_attr.json"
)

// LoadEquipSlotsFromJSONFile 读取部位表。
func LoadEquipSlotsFromJSONFile(path string) ([]schema.CfgEquipSlot, error) {
	raw, err := loadJSONRows(path)
	if err != nil {
		return nil, err
	}
	out := make([]schema.CfgEquipSlot, 0, len(raw))
	seen := map[int32]struct{}{}
	for _, row := range raw {
		id, _ := row["id"].(float64)
		name, _ := row["name"].(string)
		group, _ := row["group_id"].(float64)
		if id < 1 || name == "" || group < 1 {
			return nil, fmt.Errorf("invalid equip slot row in %s", path)
		}
		if _, ok := seen[int32(id)]; ok {
			return nil, fmt.Errorf("duplicate equip slot %d", int(id))
		}
		seen[int32(id)] = struct{}{}
		out = append(out, schema.CfgEquipSlot{ID: int32(id), Name: name, GroupID: int32(group)})
	}
	return out, nil
}

// StatRule 是导入装备属性时用来拒绝未结算属性的名单。
type StatRule struct {
	AllowFlat    bool
	AllowPercent bool
	Settle       bool
}

// LoadEquipAttrsFromJSONFile 读取装备属性。未结算或模式不允许的行返回错误。
func LoadEquipAttrsFromJSONFile(path string, stats map[string]StatRule) ([]schema.CfgEquipAttr, error) {
	raw, err := loadJSONRows(path)
	if err != nil {
		return nil, err
	}
	out := make([]schema.CfgEquipAttr, 0, len(raw))
	for _, row := range raw {
		id, _ := row["id"].(float64)
		itemID, _ := row["item_id"].(float64)
		stat, _ := row["stat"].(string)
		mode, _ := row["mode"].(string)
		value, _ := row["value"].(float64)
		if id < 1 || itemID < 1 || stat == "" || value == 0 {
			return nil, fmt.Errorf("invalid equip attr row in %s", path)
		}
		rule, ok := stats[stat]
		if !ok || !rule.Settle {
			return nil, fmt.Errorf("equip attr %d uses unsettled stat %s", int(id), stat)
		}
		switch mode {
		case "flat":
			if !rule.AllowFlat {
				return nil, fmt.Errorf("equip attr %d stat %s rejects flat", int(id), stat)
			}
		case "percent":
			if !rule.AllowPercent {
				return nil, fmt.Errorf("equip attr %d stat %s rejects percent", int(id), stat)
			}
		default:
			return nil, fmt.Errorf("equip attr %d bad mode %s", int(id), mode)
		}
		out = append(out, schema.CfgEquipAttr{
			ID: int32(id), ItemID: int32(itemID), Stat: stat, Mode: mode, Value: int32(value),
		})
	}
	return out, nil
}
