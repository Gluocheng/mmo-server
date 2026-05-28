package persistence

import "time"

type Account struct {
	UID       int64     `gorm:"column:uid;primaryKey;autoIncrement"`
	Nickname  string    `gorm:"column:nickname;size:64;uniqueIndex;not null"`
	Password  string    `gorm:"column:password;size:128;not null"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (Account) TableName() string {
	return "accounts"
}
