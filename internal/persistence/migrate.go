package persistence

import (
	"errors"

	"gorm.io/gorm"

	"github.com/example/mmo-server/gameconfig/pkg/schema"
)

func autoMigrateModels(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&Account{},
		&Player{},
		&IDSequence{},
		&InventoryItem{},
	); err != nil {
		return err
	}
	if err := db.AutoMigrate(schema.Models()...); err != nil {
		return err
	}
	if err := initializePlayerIDSequence(db); err != nil {
		return err
	}
	return migrateInventorySlots(db)
}

// initializePlayerIDSequence 按历史最大角色 ID 初始化短数字序列，避免迁移后新角色撞号。
func initializePlayerIDSequence(db *gorm.DB) error {
	var maxPlayerID int64
	if err := db.Model(&Player{}).Select("COALESCE(MAX(player_id), 0)").Scan(&maxPlayerID).Error; err != nil {
		return err
	}
	nextValue := maxPlayerID + 1
	if nextValue < playerIDInitialValue {
		nextValue = playerIDInitialValue
	}

	var seq IDSequence
	err := db.Where("name = ?", sequencePlayerID).First(&seq).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return db.Create(&IDSequence{Name: sequencePlayerID, NextValue: nextValue}).Error
	}
	if err != nil {
		return err
	}
	if seq.NextValue < nextValue {
		return db.Model(&seq).Update("next_value", nextValue).Error
	}
	return nil
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
