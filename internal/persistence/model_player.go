package persistence

import "time"

type Player struct {
	PlayerID  int64     `gorm:"column:player_id;primaryKey;autoIncrement"`
	UID       int64     `gorm:"column:uid;uniqueIndex;not null"`
	Name      string    `gorm:"column:name;size:32;not null"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (Player) TableName() string {
	return "players"
}
