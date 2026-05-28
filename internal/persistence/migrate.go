package persistence

import "gorm.io/gorm"

func autoMigrateModels(db *gorm.DB) error {
	return db.AutoMigrate(
		&Account{},
		&Player{},
		&InventoryItem{},
	)
}
