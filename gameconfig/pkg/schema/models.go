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

// CfgItem 道具静态配置行（与 Luban Item / gen/cfg.Item 同构）。UseBuffID 为 0 表示不能使用。
type CfgItem struct {
	ID          int32  `gorm:"primaryKey"`
	Name        string `gorm:"size:64;not null"`
	Type        string `gorm:"size:32;not null"`
	MaxStack    int32  `gorm:"not null"`
	Stackable   bool   `gorm:"not null"`
	Discardable bool   `gorm:"not null;default:true"`
	BindType    string `gorm:"size:16;not null;default:none"`
	BagType     int32  `gorm:"not null;default:1"`
	UseBuffID   int32  `gorm:"column:use_buff_id;not null;default:0"`
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
	Factor     int32  `gorm:"not null;default:0"`
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
	Stat       string `gorm:"size:32;not null;default:''"`
	Mode       string `gorm:"size:16;not null;default:''"`
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
	Attack        int32 `gorm:"not null;default:0"`
	Defense       int32 `gorm:"not null;default:0"`
}

// TableName 表名 cfg_combat_const。
func (CfgCombatConst) TableName() string { return "cfg_combat_const" }

// CfgScene 地图静态配置。main 全表只能有一行 true。max_lines 小于 2 表示不能分线。
type CfgScene struct {
	ID          int32   `gorm:"primaryKey"`
	Name        string  `gorm:"size:64;not null"`
	Main        bool    `gorm:"not null"`
	AllowCombat bool    `gorm:"not null"`
	MaxOnline   int32   `gorm:"not null"`
	MaxLines    int32   `gorm:"not null"`
	SpawnX      float32 `gorm:"not null"`
	SpawnY      float32 `gorm:"not null"`
	SpawnZ      float32 `gorm:"not null"`
	SwitchCdMs  int32   `gorm:"not null"`
}

// TableName 表名 cfg_scene。
func (CfgScene) TableName() string { return "cfg_scene" }

// CfgMonster 怪物模板。生成实例时拷贝这些数值。
type CfgMonster struct {
	ID               int32   `gorm:"primaryKey"`
	Name             string  `gorm:"size:64;not null"`
	HP               int32   `gorm:"column:hp;not null"`
	Attack           int32   `gorm:"not null"`
	Defense          int32   `gorm:"not null"`
	MoveSpeed        float32 `gorm:"column:move_speed;not null"`
	AttackRange      float32 `gorm:"column:attack_range;not null"`
	AttackIntervalMs int32   `gorm:"column:attack_interval_ms;not null"`
	AggroRange       float32 `gorm:"column:aggro_range;not null"`
	LeashRange       float32 `gorm:"column:leash_range;not null"`
	Kind             string  `gorm:"size:16;not null;default:''"`
}

// TableName 表名 cfg_monster。
func (CfgMonster) TableName() string { return "cfg_monster" }

// CfgSpawn 刷怪点。一条点按数量生成多只，每只有自己的出生点。
type CfgSpawn struct {
	ID        int32   `gorm:"primaryKey"`
	Feature   string  `gorm:"size:32;not null"`
	SceneID   int32   `gorm:"column:scene_id;not null"`
	Line      int32   `gorm:"not null"`
	MonsterID int32   `gorm:"column:monster_id;not null"`
	X         float32 `gorm:"not null"`
	Y         float32 `gorm:"not null"`
	Z         float32 `gorm:"not null"`
	Count     int32   `gorm:"not null"`
	RespawnMs int32   `gorm:"column:respawn_ms;not null"`
}

// TableName 表名 cfg_spawn。
func (CfgSpawn) TableName() string { return "cfg_spawn" }

// CfgKillReward 击杀奖励。monster_id + kind 唯一。kind 为 solo、party、rank1、rank2、rank3、last。
type CfgKillReward struct {
	ID        int32  `gorm:"primaryKey"`
	MonsterID int32  `gorm:"column:monster_id;not null;uniqueIndex:uk_kill_reward"`
	Kind      string `gorm:"size:16;not null;uniqueIndex:uk_kill_reward"`
	ItemID    int32  `gorm:"column:item_id;not null"`
	Count     int32  `gorm:"not null"`
}

// TableName 表名 cfg_kill_reward。
func (CfgKillReward) TableName() string { return "cfg_kill_reward" }

// CfgStat 属性名单。settle 为 false 的行只登记，挂到 Buff 上时无效。
type CfgStat struct {
	ID           int32  `gorm:"primaryKey"`
	Name         string `gorm:"size:32;not null;uniqueIndex"`
	AllowFlat    bool   `gorm:"not null"`
	AllowPercent bool   `gorm:"not null"`
	Settle       bool   `gorm:"not null"`
}

// TableName 表名 cfg_stat。
func (CfgStat) TableName() string { return "cfg_stat" }

// Models 参与 AutoMigrate 的配置表模型列表。
func Models() []any {
	return []any{&CfgVersion{}, &CfgItem{}, &CfgBagType{}, &CfgSkill{}, &CfgBuff{}, &CfgCombatConst{}, &CfgScene{}, &CfgMonster{}, &CfgSpawn{}, &CfgKillReward{}, &CfgStat{}}
}
