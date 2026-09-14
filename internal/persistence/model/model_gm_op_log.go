package model

import "time"

// GMOpLog 记录运营写操作（grant / kick / reload）；查询不写本表。
type GMOpLog struct {
	ID             int64     `gorm:"column:id;primaryKey;autoIncrement"`
	Operator       string    `gorm:"column:operator;size:64;not null;index:idx_gm_ops_operator"`
	Action         string    `gorm:"column:action;size:32;not null"`
	TargetUID      int64     `gorm:"column:target_uid"`
	TargetPlayerID int64     `gorm:"column:target_player_id;index:idx_gm_ops_player"`
	Detail         string    `gorm:"column:detail;size:1024"`
	ResultCode     int32     `gorm:"column:result_code"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime;index:idx_gm_ops_created_at"`
}

func (GMOpLog) TableName() string {
	return "gm_ops_logs"
}
