package persistence

import "time"

// Account 是登录账号模型；uid 由服务端生成，不能依赖数据库自增。
type Account struct {
	UID       int64     `gorm:"column:uid;primaryKey;autoIncrement:false"`
	Nickname  string    `gorm:"column:nickname;size:64;uniqueIndex;not null"`
	Password  string    `gorm:"column:password;size:128;not null"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (Account) TableName() string {
	return "accounts"
}
