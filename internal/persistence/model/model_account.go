package model

import "time"

// Account 是登录账号模型；uid 由服务端生成，不能依赖数据库自增。
type Account struct {
	UID         int64     `gorm:"column:uid;primaryKey;autoIncrement:false"`
	Nickname    string    `gorm:"column:nickname;size:64;uniqueIndex;not null"`
	Password    string    `gorm:"column:password;size:128;not null"`
	Banned      bool      `gorm:"column:banned;not null;default:false"`
	BanReason   string    `gorm:"column:ban_reason;size:256"`
	BannedUntil int64     `gorm:"column:banned_until;not null;default:0"` // 到期 unix 秒；0=永久
	Muted       bool      `gorm:"column:muted;not null;default:false"`
	MuteReason  string    `gorm:"column:mute_reason;size:256"`
	MutedUntil  int64     `gorm:"column:muted_until;not null;default:0"` // 到期 unix 秒；0=永久
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (Account) TableName() string {
	return "accounts"
}
