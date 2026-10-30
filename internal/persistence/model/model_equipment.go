package model

// PlayerEquipment 是角色当前穿着的一个部位。空部位不写行。
type PlayerEquipment struct {
	ID       int64 `gorm:"primaryKey"`
	PlayerID int64 `gorm:"not null;uniqueIndex:idx_player_equip,priority:1"`
	SlotID   int32 `gorm:"not null;uniqueIndex:idx_player_equip,priority:2"`
	ItemID   int32 `gorm:"not null"`
}

// TableName 表名 player_equipment。
func (PlayerEquipment) TableName() string { return "player_equipment" }
