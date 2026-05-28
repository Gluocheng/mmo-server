package persistence

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func resetDBForTest(t *testing.T) *gorm.DB {
	t.Helper()
	gdb, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := autoMigrateModels(gdb); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	oldDB := db
	oldRDB := rdb
	oldInitErr := initErr
	hadInit := oldDB != nil
	db = gdb
	rdb = nil
	once = sync.Once{}
	initErr = nil

	t.Cleanup(func() {
		db = oldDB
		rdb = oldRDB
		initErr = oldInitErr
		once = sync.Once{}
		if hadInit {
			once.Do(func() {})
		}
	})
	return gdb
}

func resetStoreForTest(t *testing.T) *gorm.DB {
	return resetDBForTest(t)
}

func TestWithinTxCommits(t *testing.T) {
	resetStoreForTest(t)
	ctx := context.Background()

	err := WithinTx(ctx, func(txCtx context.Context) error {
		return DBFromContext(txCtx).WithContext(txCtx).Create(&Account{
			Nickname: "alice",
			Password: "hash",
		}).Error
	})
	if err != nil {
		t.Fatalf("WithinTx: %v", err)
	}

	var count int64
	if err := db.Model(&Account{}).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 account, got %d", count)
	}
}

func TestWithinTxRollsBack(t *testing.T) {
	resetStoreForTest(t)
	ctx := context.Background()

	err := WithinTx(ctx, func(txCtx context.Context) error {
		if err := DBFromContext(txCtx).WithContext(txCtx).Create(&Account{
			Nickname: "bob",
			Password: "hash",
		}).Error; err != nil {
			return err
		}
		return errors.New("force rollback")
	})
	if err == nil {
		t.Fatal("expected rollback error")
	}

	var count int64
	if err := db.Model(&Account{}).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 accounts after rollback, got %d", count)
	}
}

func TestAfterCommitRunsOnSuccess(t *testing.T) {
	resetStoreForTest(t)
	ctx := context.Background()
	called := false

	err := WithinTx(ctx, func(txCtx context.Context) error {
		AfterCommit(txCtx, func(context.Context) {
			called = true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("WithinTx: %v", err)
	}
	if !called {
		t.Fatal("after commit hook was not executed")
	}
}

func TestAfterCommitSkippedOnRollback(t *testing.T) {
	resetStoreForTest(t)
	ctx := context.Background()
	called := false

	_ = WithinTx(ctx, func(txCtx context.Context) error {
		AfterCommit(txCtx, func(context.Context) {
			called = true
		})
		return errors.New("rollback")
	})
	if called {
		t.Fatal("after commit hook should not run on rollback")
	}
}

func TestNestedWithinTxReusesTransaction(t *testing.T) {
	resetStoreForTest(t)
	ctx := context.Background()

	err := WithinTx(ctx, func(outer context.Context) error {
		return WithinTx(outer, func(inner context.Context) error {
			return DBFromContext(inner).WithContext(inner).Create(&Account{
				Nickname: "nested",
				Password: "hash",
			}).Error
		})
	})
	if err != nil {
		t.Fatalf("nested WithinTx: %v", err)
	}

	var count int64
	if err := db.Model(&Account{}).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 account, got %d", count)
	}
}

func TestFindOrCreateAccountInTxCreatesAndFinds(t *testing.T) {
	resetStoreForTest(t)
	ctx := context.Background()

	var uid int64
	if err := WithinTx(ctx, func(txCtx context.Context) error {
		var err error
		uid, err = findOrCreateAccountInTx(txCtx, "hero", "secret")
		return err
	}); err != nil {
		t.Fatalf("create account: %v", err)
	}
	if uid < 1 {
		t.Fatalf("invalid uid: %d", uid)
	}

	if err := WithinTx(ctx, func(txCtx context.Context) error {
		got, err := findOrCreateAccountInTx(txCtx, "hero", "secret")
		if err != nil {
			return err
		}
		if got != uid {
			t.Fatalf("expected uid %d, got %d", uid, got)
		}
		return nil
	}); err != nil {
		t.Fatalf("find account: %v", err)
	}
}

func TestFindOrCreateAccountInTxInvalidPassword(t *testing.T) {
	resetStoreForTest(t)
	ctx := context.Background()

	if err := WithinTx(ctx, func(txCtx context.Context) error {
		_, err := findOrCreateAccountInTx(txCtx, "hero", "secret")
		return err
	}); err != nil {
		t.Fatalf("create account: %v", err)
	}

	err := WithinTx(ctx, func(txCtx context.Context) error {
		_, err := findOrCreateAccountInTx(txCtx, "hero", "wrong")
		return err
	})
	if !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("expected ErrInvalidPassword, got %v", err)
	}
}

func TestCreatePlayerInTxDuplicateUID(t *testing.T) {
	resetStoreForTest(t)
	ctx := context.Background()

	if err := WithinTx(ctx, func(txCtx context.Context) error {
		_, created, err := createPlayerInTx(txCtx, 1001, "Knight")
		if err != nil || !created {
			t.Fatalf("first create failed created=%v err=%v", created, err)
		}
		return nil
	}); err != nil {
		t.Fatalf("first WithinTx: %v", err)
	}

	if err := WithinTx(ctx, func(txCtx context.Context) error {
		_, created, err := createPlayerInTx(txCtx, 1001, "Knight2")
		if err != nil {
			return err
		}
		if created {
			t.Fatal("duplicate uid should not create again")
		}
		return nil
	}); err != nil {
		t.Fatalf("second WithinTx: %v", err)
	}
}
