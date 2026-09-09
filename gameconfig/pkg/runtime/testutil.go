package runtime

import (
	"context"

	"github.com/example/mmo-server/gameconfig/gen/cfg"
	"github.com/example/mmo-server/gameconfig/pkg/schema"
	"gorm.io/gorm"
)

// SeedTestItems 写入测试用 cfg 行并 Load（单测：覆盖 bag 用 item_id 1..1100）。
func SeedTestItems(ctx context.Context, db *gorm.DB) error {
	if err := db.WithContext(ctx).AutoMigrate(schema.Models()...); err != nil {
		return err
	}
	rows := make([]schema.CfgItem, 0, 1100)
	for id := int32(1); id <= 1100; id++ {
		rows = append(rows, schema.CfgItem{
			ID:          id,
			Name:        "test-item",
			Type:        "material",
			MaxStack:    9999,
			Stackable:   true,
			Discardable: true,
			BindType:    "none",
			BagType:     1,
		})
	}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&schema.CfgItem{}).Error; err != nil {
			return err
		}
		if err := tx.CreateInBatches(rows, 200).Error; err != nil {
			return err
		}
		if err := seedBagTypesTx(tx); err != nil {
			return err
		}
		var ver schema.CfgVersion
		if err := tx.First(&ver, 1).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return tx.Create(&schema.CfgVersion{ID: 1, Version: 1}).Error
			}
			return err
		}
		return tx.Model(&ver).Update("version", ver.Version+1).Error
	})
	if err != nil {
		return err
	}
	return Load(ctx, db)
}

// SeedDemoItems 写入演示道具并 Load。
func SeedDemoItems(ctx context.Context, db *gorm.DB) error {
	items := []*cfg.ItemItem{
		{Id: 1001, Name: "小型生命药水", Type: "consumable", MaxStack: 99, Stackable: true, Discardable: true, BindType: "none", BagType: 2},
		{Id: 1002, Name: "铜币袋", Type: "material", MaxStack: 9999, Stackable: true, Discardable: true, BindType: "none", BagType: 3},
		{Id: 2001, Name: "新手木剑", Type: "equipment", MaxStack: 1, Stackable: false, Discardable: true, BindType: "none", BagType: 4},
		{Id: 3001, Name: "任务信件", Type: "quest", MaxStack: 1, Stackable: false, Discardable: false, BindType: "none", BagType: 5},
	}
	if err := db.WithContext(ctx).AutoMigrate(schema.Models()...); err != nil {
		return err
	}
	schemaRows := make([]schema.CfgItem, 0, len(items))
	for _, it := range items {
		schemaRows = append(schemaRows, schema.CfgItem{
			ID: it.Id, Name: it.Name, Type: it.Type, MaxStack: it.MaxStack,
			Stackable: it.Stackable, Discardable: it.Discardable, BindType: it.BindType,
			BagType: it.BagType,
		})
	}
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&schema.CfgItem{}).Error; err != nil {
			return err
		}
		if err := tx.Create(&schemaRows).Error; err != nil {
			return err
		}
		if err := seedBagTypesTx(tx); err != nil {
			return err
		}
		return tx.Save(&schema.CfgVersion{ID: 1, Version: 1}).Error
	}); err != nil {
		return err
	}
	return Load(ctx, db)
}

// seedBagTypesTx 写入演示/测试用的背包类型配置行。
func seedBagTypesTx(tx *gorm.DB) error {
	bagTypes := []schema.CfgBagType{
		{ID: 1, Name: "通用背包", SlotCount: 32},
		{ID: 2, Name: "消耗品", SlotCount: 32},
		{ID: 3, Name: "材料", SlotCount: 32},
		{ID: 4, Name: "装备", SlotCount: 8},
		{ID: 5, Name: "任务", SlotCount: 32},
	}
	if err := tx.Where("1 = 1").Delete(&schema.CfgBagType{}).Error; err != nil {
		return err
	}
	return tx.Create(&bagTypes).Error
}
