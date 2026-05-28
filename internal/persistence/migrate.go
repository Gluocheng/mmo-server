package persistence

import (
	"gorm.io/gorm"
)

func autoMigrateModels(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&Account{},
		&Player{},
		&InventoryItem{},
	); err != nil {
		return err
	}
	return migrateInventorySlots(db)
}

// migrateInventorySlots 将 v1（无 slot / 同 player 多行 slot=0）数据迁移为按 slot 递增。
func migrateInventorySlots(db *gorm.DB) error {
	var items []InventoryItem
	if err := db.Order("player_id asc, id asc").Find(&items).Error; err != nil {
		return err
	}
	nextSlot := make(map[int64]int32)
	for i := range items {
		item := &items[i]
		slot := nextSlot[item.PlayerID]
		if item.Slot == slot {
			nextSlot[item.PlayerID] = slot + 1
			continue
		}
		if err := db.Model(item).Update("slot", slot).Error; err != nil {
			return err
		}
		nextSlot[item.PlayerID] = slot + 1
	}
	return nil
}
