package model

import "time"

// RewardClaim 是背包放不下时留下的待领取。领取成功后删除。
type RewardClaim struct {
	ID        int64     `gorm:"column:id;primaryKey;autoIncrement"`
	PlayerID  int64     `gorm:"column:player_id;index;not null"`
	ItemID    int32     `gorm:"column:item_id;not null"`
	Count     int32     `gorm:"column:count;not null"`
	Reason    string    `gorm:"column:reason;size:16;not null"`
	MonsterID int32     `gorm:"column:monster_id;not null"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
}

// TableName 表名 player_reward_claims。
func (RewardClaim) TableName() string { return "player_reward_claims" }
