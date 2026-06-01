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
		})
	}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&schema.CfgItem{}).Error; err != nil {
			return err
		}
		if err := tx.CreateInBatches(rows, 200).Error; err != nil {
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
	items := []*cfg.Item{
		{ID: 1001, Name: "小型生命药水", Type: "consumable", MaxStack: 99, Stackable: true, Discardable: true, BindType: "none"},
		{ID: 1002, Name: "铜币袋", Type: "material", MaxStack: 9999, Stackable: true, Discardable: true, BindType: "none"},
		{ID: 2001, Name: "新手木剑", Type: "equipment", MaxStack: 1, Stackable: false, Discardable: true, BindType: "none"},
		{ID: 3001, Name: "任务信件", Type: "quest", MaxStack: 1, Stackable: false, Discardable: false, BindType: "none"},
	}
	if err := db.WithContext(ctx).AutoMigrate(schema.Models()...); err != nil {
		return err
	}
	schemaRows := make([]schema.CfgItem, 0, len(items))
	for _, it := range items {
		schemaRows = append(schemaRows, schema.CfgItem{
			ID: it.ID, Name: it.Name, Type: it.Type, MaxStack: it.MaxStack,
			Stackable: it.Stackable, Discardable: it.Discardable, BindType: it.BindType,
		})
	}
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&schema.CfgItem{}).Error; err != nil {
			return err
		}
		if err := tx.Create(&schemaRows).Error; err != nil {
			return err
		}
		return tx.Save(&schema.CfgVersion{ID: 1, Version: 1}).Error
	}); err != nil {
		return err
	}
	return Load(ctx, db)
}
