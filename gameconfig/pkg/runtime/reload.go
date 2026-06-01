package runtime

import (
	"context"

	"gorm.io/gorm"
)

// Reload 从 MySQL 重新加载；失败时保留旧快照。
func Reload(ctx context.Context, db *gorm.DB) error {
	return Load(ctx, db)
}
