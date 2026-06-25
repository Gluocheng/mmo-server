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
	if err := downgradePlayerUIDUniqueIndex(db); err != nil {
		return err
	}
	return migrateInventorySlots(db)
}

// downgradePlayerUIDUniqueIndex 将 players.uid 由唯一索引降级为普通索引，支持一账号多角。
// GORM AutoMigrate 不会自动删除既有索引，这里显式处理；索引不存在时忽略错误。
func downgradePlayerUIDUniqueIndex(db *gorm.DB) error {
	migrator := db.Migrator()
	// 旧唯一索引名遵循 GORM 默认命名：idx_players_uid
	if migrator.HasIndex(&Player{}, "idx_players_uid") {
		_ = migrator.DropIndex(&Player{}, "idx_players_uid")
	}
	// 部分历史库可能为 uni_players_uid，一并尝试清理
	if migrator.HasIndex(&Player{}, "uni_players_uid") {
		_ = migrator.DropIndex(&Player{}, "uni_players_uid")
	}
	return migrator.CreateIndex(&Player{}, "idx_players_uid")
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
