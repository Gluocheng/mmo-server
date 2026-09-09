package model

import "time"

// Player 是角色模型；同账号可持有多个未删除角色，player_id 全局短数字序列分配。
type Player struct {
	PlayerID  int64      `gorm:"column:player_id;primaryKey;autoIncrement:false"`
	UID       int64      `gorm:"column:uid;index;not null"`
	Name      string     `gorm:"column:name;size:32;not null"`
	DeletedAt *time.Time `gorm:"column:deleted_at;index"`
	CreatedAt time.Time  `gorm:"column:created_at;autoCreateTime"`
}

func (Player) TableName() string {
	return "players"
}
