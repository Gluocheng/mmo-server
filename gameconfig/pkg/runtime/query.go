package runtime

import (
	"github.com/example/mmo-server/gameconfig/gen/cfg"
)

// ItemDef 对外暴露的道具配置视图。
type ItemDef struct {
	ID          int32
	Name        string
	Type        string
	MaxStack    int32
	Stackable   bool
	Discardable bool
	BindType    string
	BagType     int32
}

func toItemDef(item *cfg.ItemItem) ItemDef {
	if item == nil {
		return ItemDef{}
	}
	return ItemDef{
		ID:          item.Id,
		Name:        item.Name,
		Type:        item.Type,
		MaxStack:    item.MaxStack,
		Stackable:   item.Stackable,
		Discardable: item.Discardable,
		BindType:    item.BindType,
		BagType:     item.BagType,
	}
}

// Exists 判断 item_id 是否在配置表中。
func Exists(itemID int32) bool {
	s := getSnapshot()
	if s == nil || s.tables == nil || s.tables.items == nil {
		return false
	}
	_, ok := s.tables.items[itemID]
	return ok
}

// Get 按 id 查询道具配置。
func Get(itemID int32) (ItemDef, bool) {
	s := getSnapshot()
	if s == nil || s.tables == nil || s.tables.items == nil {
		return ItemDef{}, false
	}
	item, ok := s.tables.items[itemID]
	if !ok {
		return ItemDef{}, false
	}
	return toItemDef(item), true
}

// MaxStack 返回道具单槽堆叠上限；不存在返回 0。
func MaxStack(itemID int32) int32 {
	def, ok := Get(itemID)
	if !ok {
		return 0
	}
	return def.MaxStack
}

// ValidateItemID 校验 item_id 存在性。
func ValidateItemID(itemID int32) error {
	if !Exists(itemID) {
		return ErrItemNotFound
	}
	return nil
}

// BagTypeExists 判断背包类型 id 是否存在。
func BagTypeExists(bagType int32) bool {
	s := getSnapshot()
	if s == nil || s.tables == nil || s.tables.bagTypes == nil {
		return false
	}
	_, ok := s.tables.bagTypes[bagType]
	return ok
}

// SlotCount 返回背包类型的槽位数；不存在返回 0。
func SlotCount(bagType int32) int32 {
	s := getSnapshot()
	if s == nil || s.tables == nil || s.tables.bagTypes == nil {
		return 0
	}
	def, ok := s.tables.bagTypes[bagType]
	if !ok {
		return 0
	}
	return def.SlotCount
}

// BagTypeName 返回背包类型名；不存在返回空串。
func BagTypeName(bagType int32) string {
	s := getSnapshot()
	if s == nil || s.tables == nil || s.tables.bagTypes == nil {
		return ""
	}
	def, ok := s.tables.bagTypes[bagType]
	if !ok {
		return ""
	}
	return def.Name
}
