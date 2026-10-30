package model

// PlayerEquipment 是角色当前穿着的一个部位。空部位不写行。
type PlayerEquipment struct {
	ID           int64 `gorm:"primaryKey"`
	PlayerID     int64 `gorm:"not null;uniqueIndex:idx_player_equip,priority:1"`
	SlotID       int32 `gorm:"not null;uniqueIndex:idx_player_equip,priority:2"`
	ItemID       int32 `gorm:"not null"`
	EnhanceLevel int32 `gorm:"column:enhance_level;not null;default:0"` // 这件装备的强化等级
}

// TableName 表名 player_equipment。
func (PlayerEquipment) TableName() string { return "player_equipment" }
