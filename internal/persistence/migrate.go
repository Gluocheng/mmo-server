package persistence

import (
	"errors"
	"strings"

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
		&model.RewardClaim{},
		&model.PlayerEquipment{},
	); err != nil {
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

// autoMigrateConfig 在配置库上建 cfg_*。玩家库不调用。
func autoMigrateConfig(db *gorm.DB) error {
	return db.AutoMigrate(schema.Models()...)
}

// downgradePlayerUIDUniqueIndex 将 players.uid 由唯一索引降级为普通索引，支持一账号多角。
// GORM AutoMigrate 不会自动删除既有索引，这里显式处理；索引不存在时忽略错误。
func downgradePlayerUIDUniqueIndex(db *gorm.DB) error {
	migrator := db.Migrator()
	// 部分历史库可能为 uni_players_uid，一并尝试清理
	if migrator.HasIndex(&model.Player{}, "uni_players_uid") {
		_ = migrator.DropIndex(&model.Player{}, "uni_players_uid")
	}
	// 内存库没有 information_schema，索引已在时直接留下。
	if db.Dialector.Name() == "sqlite" {
		if migrator.HasIndex(&model.Player{}, "idx_players_uid") {
			return nil
		}
		return migrator.CreateIndex(&model.Player{}, "idx_players_uid")
	}
	// 已经是普通索引时不再删除重建。两个登录进程同时迁移时，删除再建会撞上 Duplicate key name。
	state, err := indexUniqueness(db, "players", "idx_players_uid")
	if err != nil {
		return err
	}
	if state == indexNonUnique {
		return nil
	}
	if state == indexUnique {
		_ = migrator.DropIndex(&model.Player{}, "idx_players_uid")
	}
	if err := migrator.CreateIndex(&model.Player{}, "idx_players_uid"); err != nil && !isDuplicateKeyName(err) {
		return err
	}
	return nil
}

const (
	indexMissing = iota
	indexUnique
	indexNonUnique
)

func indexUniqueness(db *gorm.DB, table, index string) (int, error) {
	var rows []struct {
		NonUnique int `gorm:"column:NON_UNIQUE"`
	}
	err := db.Raw(
		`SELECT NON_UNIQUE FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND INDEX_NAME = ? LIMIT 1`,
		table, index,
	).Scan(&rows).Error
	if err != nil {
		return indexMissing, err
	}
	if len(rows) == 0 {
		return indexMissing, nil
	}
	if rows[0].NonUnique == 0 {
		return indexUnique, nil
	}
	return indexNonUnique, nil
}

func isDuplicateKeyName(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "Duplicate key name") || strings.Contains(msg, "Error 1061")
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

// migrateInventorySlots 将 v1（无 slot / 同 player 多行 slot=0）数据迁移为按 slot 递增。
// 玩家库里若还留着拆库前的 cfg_item，按它回填 bag_type。表不在，或道具未声明类型时，回填通用背包(1)。
func migrateInventorySlots(db *gorm.DB) error {
	bagTypeByItem, err := loadBagTypeByItem(db)
	if err != nil {
		return err
	}

	var items []model.InventoryItem
	if err := db.Order("player_id asc, id asc").Find(&items).Error; err != nil {
		return err
	}
	// 唯一索引已在则槽位已合法；再按 player 压槽会与 (player,bag,slot) 冲突。
	hasSlotUnique := db.Migrator().HasIndex(&model.InventoryItem{}, "idx_player_bag_slot")
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
		if !hasSlotUnique && item.Slot != slot {
			updates["slot"] = slot
		}
		if len(updates) > 0 {
			if err := db.Model(item).Updates(updates).Error; err != nil {
				return err
			}
		}
		if hasSlotUnique {
			continue
		}
		nextSlot[item.PlayerID] = slot + 1
	}
	return nil
}

// loadBagTypeByItem 从玩家库残留的 cfg_item 读取 item_id -> bag_type（仅 BagType>0）。
// 配置已经拆走、这张表不在时返回空映射。
func loadBagTypeByItem(db *gorm.DB) (map[int32]int32, error) {
	if !db.Migrator().HasTable(&schema.CfgItem{}) {
		return map[int32]int32{}, nil
	}
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
