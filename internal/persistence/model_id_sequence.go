package persistence

import "time"

// IDSequence 保存需要全局短 ID 的业务序列，next_value 表示下一次可分配值。
type IDSequence struct {
	Name      string    `gorm:"column:name;size:64;primaryKey"`
	NextValue int64     `gorm:"column:next_value;not null"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (IDSequence) TableName() string {
	return "id_sequences"
}
