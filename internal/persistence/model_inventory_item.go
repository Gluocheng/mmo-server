package persistence

// InventoryItem 背包槽位行：每个 (player_id, slot) 至多一条记录。
type InventoryItem struct {
	ID       int64 `gorm:"column:id;primaryKey;autoIncrement"`
	PlayerID int64 `gorm:"column:player_id;uniqueIndex:idx_player_slot;not null"`
	Slot     int32 `gorm:"column:slot;uniqueIndex:idx_player_slot;not null"` // 槽位 0..MaxBagSlots-1
	ItemID   int32 `gorm:"column:item_id;not null"`
	Count    int32 `gorm:"column:count;not null"`
}

func (InventoryItem) TableName() string {
	return "inventory_items"
}
