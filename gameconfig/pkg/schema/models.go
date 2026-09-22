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
	BagType     int32  `gorm:"not null;default:1"`
}

// TableName 表名 cfg_item。
func (CfgItem) TableName() string { return "cfg_item" }

// CfgBagType 背包类型静态配置行（与 Luban BagType / gen/cfg.BagType 同构）。
type CfgBagType struct {
	ID        int32  `gorm:"primaryKey"`
	Name      string `gorm:"size:64;not null"`
	SlotCount int32  `gorm:"not null"`
}

// TableName 表名 cfg_bag_type。
func (CfgBagType) TableName() string { return "cfg_bag_type" }

// CfgSkill 技能静态配置。target 为 single（点名）或 aoe_self（自身圆心）。
type CfgSkill struct {
	ID         int32  `gorm:"primaryKey"`
	Name       string `gorm:"size:64;not null"`
	Target     string `gorm:"size:16;not null"`
	CastRange  int32  `gorm:"column:cast_range;not null"`
	Radius     int32  `gorm:"not null"`
	CooldownMs int32  `gorm:"not null"`
	Damage     int32  `gorm:"not null"`
	BuffID     int32  `gorm:"not null"`
}

// TableName 表名 cfg_skill。
func (CfgSkill) TableName() string { return "cfg_skill" }

// CfgBuff 持续效果。effect 为 dot（伤害）或 hot（治疗）。
type CfgBuff struct {
	ID         int32  `gorm:"primaryKey"`
	Name       string `gorm:"size:64;not null"`
	DurationMs int32  `gorm:"not null"`
	IntervalMs int32  `gorm:"not null"`
	Effect     string `gorm:"size:16;not null"`
	Value      int32  `gorm:"not null"`
	MaxStack   int32  `gorm:"not null"`
}

// TableName 表名 cfg_buff。
func (CfgBuff) TableName() string { return "cfg_buff" }

// CfgCombatConst 战斗常量，固定 id=1。
type CfgCombatConst struct {
	ID            int32 `gorm:"primaryKey"`
	MaxHP         int32 `gorm:"column:max_hp;not null"`
	TickMs        int32 `gorm:"not null"`
	RespawnMs     int32 `gorm:"not null"`
	FrameEventCap int32 `gorm:"not null"`
}

// TableName 表名 cfg_combat_const。
func (CfgCombatConst) TableName() string { return "cfg_combat_const" }

// Models 参与 AutoMigrate 的配置表模型列表。
func Models() []any {
	return []any{&CfgVersion{}, &CfgItem{}, &CfgBagType{}, &CfgSkill{}, &CfgBuff{}, &CfgCombatConst{}}
}
