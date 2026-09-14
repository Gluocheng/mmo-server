package persistence

import (
	"errors"

	"github.com/example/mmo-server/gameconfig/pkg/schema"
	"github.com/example/mmo-server/internal/persistence/model"
	"gorm.io/gorm"
)

func autoMigrateModels(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&model.Account{},
		&model.Player{},
		&model.IDSequence{},
		&model.InventoryItem{},
		&model.GMOpLog{},
		&model.GMUser{},
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
	if err := downgradeInventorySlotUniqueIndex(db); err != nil {
		return err
	}
	return migrateInventorySlots(db)
}

// downgradePlayerUIDUniqueIndex 将 players.uid 由唯一索引降级为普通索引，支持一账号多角。
// GORM AutoMigrate 不会自动删除既有索引，这里显式处理；索引不存在时忽略错误。
func downgradePlayerUIDUniqueIndex(db *gorm.DB) error {
	migrator := db.Migrator()
	// 旧唯一索引名遵循 GORM 默认命名：idx_players_uid
	if migrator.HasIndex(&model.Player{}, "idx_players_uid") {
		_ = migrator.DropIndex(&model.Player{}, "idx_players_uid")
	}
	// 部分历史库可能为 uni_players_uid，一并尝试清理
	if migrator.HasIndex(&model.Player{}, "uni_players_uid") {
		_ = migrator.DropIndex(&model.Player{}, "uni_players_uid")
	}
	return migrator.CreateIndex(&model.Player{}, "idx_players_uid")
}

// initializePlayerIDSequence 按历史最大角色 ID 初始化短数字序列，避免迁移后新角色撞号。
func initializePlayerIDSequence(db *gorm.DB) error {
	var maxPlayerID int64
	if err := db.Model(&model.Player{}).Select("COALESCE(MAX(player_id), 0)").Scan(&maxPlayerID).Error; err != nil {
		return err
	}
	nextValue := maxPlayerID + 1
	if nextValue < playerIDInitialValue {
		nextValue = playerIDInitialValue
	}

	var seq model.IDSequence
	err := db.Where("name = ?", sequencePlayerID).First(&seq).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return db.Create(&model.IDSequence{Name: sequencePlayerID, NextValue: nextValue}).Error
	}
	if err != nil {
		return err
	}
	if seq.NextValue < nextValue {
		return db.Model(&seq).Update("next_value", nextValue).Error
	}
	return nil
}

// migrateInventorySlots 将 v1（无 slot / 同 player 多行 slot=0）数据迁移为按 slot 递增，
// 并按 cfg_item 表精确回填 bag_type。迁移早于 gameconfig 内存快照加载，故直接查 cfg_item 表
// 而非依赖 runtime 快照；item 不在配表或配表未声明 bag_type 时回填通用背包(1)。
func migrateInventorySlots(db *gorm.DB) error {
	bagTypeByItem, err := loadBagTypeByItem(db)
	if err != nil {
		return err
	}

	var items []model.InventoryItem
	if err := db.Order("player_id asc, id asc").Find(&items).Error; err != nil {
		return err
	}
	nextSlot := make(map[int64]int32)
	for i := range items {
		item := &items[i]
		slot := nextSlot[item.PlayerID]
		updates := map[string]interface{}{}
		if item.BagType < 1 {
			bt := GeneralBagType
			if v, ok := bagTypeByItem[item.ItemID]; ok {
				bt = v
			}
			updates["bag_type"] = bt
		}
		if item.Slot != slot {
			updates["slot"] = slot
		}
		if len(updates) > 0 {
			if err := db.Model(item).Updates(updates).Error; err != nil {
				return err
			}
		}
		nextSlot[item.PlayerID] = slot + 1
	}
	return nil
}

// loadBagTypeByItem 从 cfg_item 表读取 item_id -> bag_type 映射（仅 BagType>0 的项）。
func loadBagTypeByItem(db *gorm.DB) (map[int32]int32, error) {
	var cfgItems []schema.CfgItem
	if err := db.Order("id asc").Find(&cfgItems).Error; err != nil {
		return nil, err
	}
	m := make(map[int32]int32, len(cfgItems))
	for i := range cfgItems {
		if cfgItems[i].BagType > 0 {
			m[cfgItems[i].ID] = cfgItems[i].BagType
		}
	}
	return m, nil
}

// downgradeInventorySlotUniqueIndex 将 inventory_items 的旧唯一索引 (player_id, slot)
// 降级为 (player_id, bag_type, slot)。AutoMigrate 不会自动删除旧唯一索引，这里显式处理。
func downgradeInventorySlotUniqueIndex(db *gorm.DB) error {
	migrator := db.Migrator()
	// 旧唯一索引名遵循 GORM 默认命名：idx_inventory_items_player_id_slot
	if migrator.HasIndex(&model.InventoryItem{}, "idx_player_slot") {
		_ = migrator.DropIndex(&model.InventoryItem{}, "idx_player_slot")
	}
	if migrator.HasIndex(&model.InventoryItem{}, "idx_inventory_items_player_id_slot") {
		_ = migrator.DropIndex(&model.InventoryItem{}, "idx_inventory_items_player_id_slot")
	}
	return nil
}
