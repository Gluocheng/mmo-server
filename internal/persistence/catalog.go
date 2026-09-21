package persistence

import "github.com/example/mmo-server/gameconfig/pkg/schema"

// ListCfgBagTypes 读取 cfg_bag_type，按 id 升序；供 GM 控制台展示类型名。
func ListCfgBagTypes() ([]schema.CfgBagType, error) {
	gdb, err := DB()
	if err != nil {
		return nil, err
	}
	var rows []schema.CfgBagType
	if err := gdb.Order("id asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// ListCfgItems 读取 cfg_item，按 id 升序；供 GM 控制台展示道具名。
func ListCfgItems() ([]schema.CfgItem, error) {
	gdb, err := DB()
	if err != nil {
		return nil, err
	}
	var rows []schema.CfgItem
	if err := gdb.Order("id asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
