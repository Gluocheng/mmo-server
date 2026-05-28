package persistence

import (
	"context"

	clog "github.com/cherry-game/cherry/logger"
	"gorm.io/gorm"
)

type txDBKey struct{}
type txHooksKey struct{}

type txHooks struct {
	callbacks []func(context.Context)
}

// WithinTx 在 MySQL 事务中执行 fn；若 ctx 已在事务内则复用当前事务。
// 事务成功提交后执行 AfterCommit 注册的回调（如 Redis 缓存刷新）。
func WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if err := ensureDB(); err != nil {
		return err
	}
	if _, ok := txDBFromContext(ctx); ok {
		return fn(ctx)
	}

	hooks := &txHooks{}
	var committed bool
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, txDBKey{}, tx)
		txCtx = context.WithValue(txCtx, txHooksKey{}, hooks)
		if err := fn(txCtx); err != nil {
			clog.Debugf("persistence: tx rollback: %v", err)
			return err
		}
		committed = true
		return nil
	})
	if err != nil {
		return err
	}
	if committed {
		runAfterCommitHooks(ctx, hooks.callbacks)
	}
	return nil
}

// DBFromContext 返回 ctx 中的事务 DB；不在事务内时返回全局 DB。
func DBFromContext(ctx context.Context) *gorm.DB {
	if txDB, ok := txDBFromContext(ctx); ok {
		return txDB
	}
	return db
}

// AfterCommit 注册事务提交后的副作用（如 Redis 缓存写入），不在事务内执行。
func AfterCommit(ctx context.Context, fn func(context.Context)) {
	if hooks, ok := ctx.Value(txHooksKey{}).(*txHooks); ok && hooks != nil {
		hooks.callbacks = append(hooks.callbacks, fn)
		return
	}
	// 非事务上下文：立即执行，便于只读路径复用缓存写入逻辑
	fn(ctx)
}

func txDBFromContext(ctx context.Context) (*gorm.DB, bool) {
	txDB, ok := ctx.Value(txDBKey{}).(*gorm.DB)
	return txDB, ok && txDB != nil
}

func ensureDB() error {
	if db != nil {
		return nil
	}
	return Init()
}

func runAfterCommitHooks(ctx context.Context, hooks []func(context.Context)) {
	for _, hook := range hooks {
		func(h func(context.Context)) {
			defer func() {
				if r := recover(); r != nil {
					clog.Errorf("persistence: after commit hook panic: %v", r)
				}
			}()
			h(ctx)
		}(hook)
	}
}
