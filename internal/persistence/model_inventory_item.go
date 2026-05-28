package persistence

type InventoryItem struct {
	ID       int64 `gorm:"column:id;primaryKey;autoIncrement"`
	PlayerID int64 `gorm:"column:player_id;uniqueIndex:idx_player_item;not null"`
	ItemID   int32 `gorm:"column:item_id;uniqueIndex:idx_player_item;not null"`
	Count    int32 `gorm:"column:count;not null"`
}

func (InventoryItem) TableName() string {
	return "inventory_items"
}
