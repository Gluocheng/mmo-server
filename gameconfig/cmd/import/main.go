package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	cprofile "github.com/cherry-game/cherry/profile"
	"github.com/example/mmo-server/gameconfig/pkg/importdata"
	"github.com/example/mmo-server/gameconfig/pkg/schema"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	profilePath := flag.String("profile", "configs/mmo-cluster.json", "cluster profile path")
	dataDir := flag.String("data-dir", "gameconfig/gen/data", "Luban JSON output directory")
	flag.Parse()

	cprofile.Init(*profilePath, "10001")
	dsn := cprofile.GetConfig("mysql").GetString("dsn", "")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "mysql.dsn is empty in profile")
		os.Exit(1)
	}

	itemPath := filepath.Join(*dataDir, importdata.ItemTableFile)
	items, err := importdata.LoadItemsFromJSONFile(itemPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	schemaRows := importdata.ItemsToSchema(items)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	gdb, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		fmt.Fprintln(os.Stderr, "open mysql:", err)
		os.Exit(1)
	}
	if err := gdb.WithContext(ctx).AutoMigrate(schema.Models()...); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}

	var newVersion int64
	err = gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&schema.CfgItem{}).Error; err != nil {
			return err
		}
		if err := tx.Create(&schemaRows).Error; err != nil {
			return err
		}
		var ver schema.CfgVersion
		if err := tx.First(&ver, 1).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				ver = schema.CfgVersion{ID: 1, Version: 1}
				return tx.Create(&ver).Error
			}
			return err
		}
		newVersion = ver.Version + 1
		return tx.Model(&ver).Updates(map[string]interface{}{
			"version": newVersion,
		}).Error
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "import:", err)
		os.Exit(1)
	}

	fmt.Printf("imported %d items, cfg_version=%d\n", len(schemaRows), newVersion)
}
