package runtime

import (
	"github.com/example/mmo-server/gameconfig/gen/cfg"
	"github.com/example/mmo-server/gameconfig/pkg/schema"
)

// EquipSlotDef 是一个可穿戴部位。
type EquipSlotDef struct {
	ID      int32
	Name    string
	GroupID int32
}

// EquipAttrDef 是一件装备上的一条属性。
type EquipAttrDef struct {
	ItemID int32
	Stat   string
	Mode   string
	Value  int32
}

// EquipSlots 按部位 id 升序返回全部部位。
func EquipSlots() []EquipSlotDef {
	s := getSnapshot()
	if s == nil || s.tables == nil || len(s.tables.equipSlots) == 0 {
		return nil
	}
	ids := make([]int32, 0, len(s.tables.equipSlots))
	for id := range s.tables.equipSlots {
		ids = append(ids, id)
	}
	sortIDs(ids)
	out := make([]EquipSlotDef, 0, len(ids))
	for _, id := range ids {
		out = append(out, s.tables.equipSlots[id])
	}
	return out
}

// EquipSlotIDsByGroup 返回这个部位组里的部位 id，按 id 升序。
func EquipSlotIDsByGroup(group int32) []int32 {
	if group < 1 {
		return nil
	}
	s := getSnapshot()
	if s == nil || s.tables == nil {
		return nil
	}
	var ids []int32
	for id, slot := range s.tables.equipSlots {
		if slot.GroupID == group {
			ids = append(ids, id)
		}
	}
	sortIDs(ids)
	return ids
}

// EquipAttrs 返回一件装备的属性行。道具不在表里时为空。
func EquipAttrs(itemID int32) []EquipAttrDef {
	s := getSnapshot()
	if s == nil || s.tables == nil {
		return nil
	}
	return s.tables.equipAttrs[itemID]
}

// BuildEquip 用内存行替换部位、装备属性和这些道具。仅测试或启动前灌入。
func BuildEquip(slots []EquipSlotDef, attrs []EquipAttrDef, items []ItemDef) {
	s := getSnapshot()
	var base *tables
	ver := int64(0)
	tc := int32(0)
	if s != nil {
		ver = s.version
		tc = s.tableCount
		base = s.tables.clone()
	} else {
		base = &tables{}
	}
	if base.items == nil {
		base.items = map[int32]*cfg.ItemItem{}
	}
	sm := make(map[int32]EquipSlotDef, len(slots))
	for _, slot := range slots {
		sm[slot.ID] = slot
	}
	am := map[int32][]EquipAttrDef{}
	for _, attr := range attrs {
		am[attr.ItemID] = append(am[attr.ItemID], attr)
	}
	gm := map[int32]int32{}
	for _, it := range items {
		maxStack := it.MaxStack
		if maxStack < 1 {
			maxStack = 1
		}
		base.items[it.ID] = &cfg.ItemItem{
			Id: it.ID, Name: it.Name, Type: it.Type, MaxStack: maxStack,
			Stackable: it.Stackable, BagType: it.BagType, UseBuffId: it.UseBuffID,
		}
		if it.EquipSlot != 0 {
			gm[it.ID] = it.EquipSlot
		}
	}
	base.equipSlots = sm
	base.equipAttrs = am
	base.itemGroup = gm
	swapSnapshot(&snapshot{version: ver, tableCount: tc, tables: base})
}

func equipSlotsFromSchema(rows []schema.CfgEquipSlot) map[int32]EquipSlotDef {
	m := make(map[int32]EquipSlotDef, len(rows))
	for _, r := range rows {
		m[r.ID] = EquipSlotDef{ID: r.ID, Name: r.Name, GroupID: r.GroupID}
	}
	return m
}

func equipAttrsFromSchema(rows []schema.CfgEquipAttr) map[int32][]EquipAttrDef {
	m := map[int32][]EquipAttrDef{}
	for _, r := range rows {
		m[r.ItemID] = append(m[r.ItemID], EquipAttrDef{ItemID: r.ItemID, Stat: r.Stat, Mode: r.Mode, Value: r.Value})
	}
	return m
}

func itemGroupsFromSchema(rows []schema.CfgItem) map[int32]int32 {
	m := make(map[int32]int32, len(rows))
	for _, r := range rows {
		if r.EquipSlot != 0 {
			m[r.ID] = r.EquipSlot
		}
	}
	return m
}

func sortIDs(ids []int32) {
	for i := 1; i < len(ids); i++ {
		j := i
		for j > 0 && ids[j] < ids[j-1] {
			ids[j], ids[j-1] = ids[j-1], ids[j]
			j--
		}
	}
}
