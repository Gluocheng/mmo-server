package persistence

import "time"

// Player 是角色模型；player_id 来自全局短数字序列，不携带服务器语义。
type Player struct {
	PlayerID  int64     `gorm:"column:player_id;primaryKey;autoIncrement:false"`
	UID       int64     `gorm:"column:uid;uniqueIndex;not null"`
	Name      string    `gorm:"column:name;size:32;not null"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (Player) TableName() string {
	return "players"
}
