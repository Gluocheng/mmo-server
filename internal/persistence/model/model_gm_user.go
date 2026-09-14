package model

import "time"

const (
	GMRoleAdmin    = "admin"
	GMRoleOperator = "operator"
)

// GMUser 是运营后台账号，与玩家 accounts 分离。
type GMUser struct {
	ID           int64     `gorm:"column:id;primaryKey;autoIncrement"`
	Username     string    `gorm:"column:username;size:64;uniqueIndex;not null"`
	PasswordHash string    `gorm:"column:password_hash;size:128;not null"`
	DisplayName  string    `gorm:"column:display_name;size:64"`
	Role         string    `gorm:"column:role;size:16;not null"`
	Disabled     bool      `gorm:"column:disabled;not null;default:false"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (GMUser) TableName() string {
	return "gm_users"
}
