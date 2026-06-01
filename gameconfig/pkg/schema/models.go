package schema

import "time"

// CfgVersion 全局配置版本元数据；固定 id=1。
type CfgVersion struct {
	ID        uint      `gorm:"primaryKey"`
	Version   int64     `gorm:"not null"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

// TableName 表名 cfg_version。
func (CfgVersion) TableName() string { return "cfg_version" }

// CfgItem 道具静态配置行（与 Luban Item / gen/cfg.Item 同构）。
type CfgItem struct {
	ID          int32  `gorm:"primaryKey"`
	Name        string `gorm:"size:64;not null"`
	Type        string `gorm:"size:32;not null"`
	MaxStack    int32  `gorm:"not null"`
	Stackable   bool   `gorm:"not null"`
	Discardable bool   `gorm:"not null;default:true"`
	BindType    string `gorm:"size:16;not null;default:none"`
}

// TableName 表名 cfg_item。
func (CfgItem) TableName() string { return "cfg_item" }

// Models 参与 AutoMigrate 的配置表模型列表。
func Models() []any {
	return []any{&CfgVersion{}, &CfgItem{}}
}
