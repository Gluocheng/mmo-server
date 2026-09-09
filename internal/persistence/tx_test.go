package persistence

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/example/mmo-server/internal/persistence/model"
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
		return DBFromContext(txCtx).WithContext(txCtx).Create(&model.Account{
			Nickname: "alice",
			Password: "hash",
		}).Error
	})
	if err != nil {
		t.Fatalf("WithinTx: %v", err)
	}

	var count int64
	if err := db.Model(&model.Account{}).Count(&count).Error; err != nil {
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
		if err := DBFromContext(txCtx).WithContext(txCtx).Create(&model.Account{
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
	if err := db.Model(&model.Account{}).Count(&count).Error; err != nil {
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
			return DBFromContext(inner).WithContext(inner).Create(&model.Account{
				Nickname: "nested",
				Password: "hash",
			}).Error
		})
	})
	if err != nil {
		t.Fatalf("nested WithinTx: %v", err)
	}

	var count int64
	if err := db.Model(&model.Account{}).Count(&count).Error; err != nil {
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

func TestFindOrCreateAccountInTxUsesGeneratedUID(t *testing.T) {
	resetStoreForTest(t)
	ctx := context.Background()

	var uid int64
	if err := WithinTx(ctx, func(txCtx context.Context) error {
		var err error
		uid, err = findOrCreateAccountInTx(txCtx, "generated", "secret")
		return err
	}); err != nil {
		t.Fatalf("create account: %v", err)
	}

	if uid <= playerIDInitialValue {
		t.Fatalf("expected generated snowflake uid, got %d", uid)
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

func TestCreatePlayerInTxAllowsMultiplePerUID(t *testing.T) {
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
		info, created, err := createPlayerInTx(txCtx, 1001, "Knight2")
		if err != nil {
			return err
		}
		if !created || info == nil {
			t.Fatal("second character for same uid should be created")
		}
		return nil
	}); err != nil {
		t.Fatalf("second WithinTx: %v", err)
	}
}

func TestAutoMigrateInitializesPlayerIDSequenceAboveHistory(t *testing.T) {
	gdb, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := gdb.AutoMigrate(&model.Player{}); err != nil {
		t.Fatalf("pre migrate players: %v", err)
	}
	if err := gdb.Create(&model.Player{PlayerID: 120345, UID: 9001, Name: "Old"}).Error; err != nil {
		t.Fatalf("seed player: %v", err)
	}

	if err := autoMigrateModels(gdb); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	var seq model.IDSequence
	if err := gdb.Where("name = ?", sequencePlayerID).First(&seq).Error; err != nil {
		t.Fatalf("load player_id sequence: %v", err)
	}
	if seq.NextValue != 120346 {
		t.Fatalf("expected sequence next value 120346, got %d", seq.NextValue)
	}
}

func TestNextPlayerIDInTxAllocatesShortSequentialIDs(t *testing.T) {
	resetStoreForTest(t)
	ctx := context.Background()

	var first, second int64
	if err := WithinTx(ctx, func(txCtx context.Context) error {
		var err error
		first, err = nextPlayerIDInTx(txCtx)
		if err != nil {
			return err
		}
		second, err = nextPlayerIDInTx(txCtx)
		return err
	}); err != nil {
		t.Fatalf("allocate player ids: %v", err)
	}

	if first != playerIDInitialValue {
		t.Fatalf("expected first player_id %d, got %d", playerIDInitialValue, first)
	}
	if second != first+1 {
		t.Fatalf("expected sequential player_id %d, got %d", first+1, second)
	}
}

func TestNextPlayerIDInTxRollsBackSequence(t *testing.T) {
	resetStoreForTest(t)
	ctx := context.Background()

	_ = WithinTx(ctx, func(txCtx context.Context) error {
		if _, err := nextPlayerIDInTx(txCtx); err != nil {
			return err
		}
		return errors.New("rollback sequence")
	})

	var got int64
	if err := WithinTx(ctx, func(txCtx context.Context) error {
		var err error
		got, err = nextPlayerIDInTx(txCtx)
		return err
	}); err != nil {
		t.Fatalf("allocate after rollback: %v", err)
	}

	if got != playerIDInitialValue {
		t.Fatalf("expected rollback to keep next player_id %d, got %d", playerIDInitialValue, got)
	}
}

// setMaxCharactersForTest 覆盖角色上限，便于多角用例构造满额场景。
func setMaxCharactersForTest(t *testing.T, n int) {
	t.Helper()
	old := cfg.maxCharacters
	cfg.maxCharacters = n
	t.Cleanup(func() { cfg.maxCharacters = old })
}

func TestListPlayersByUIDReturnsOnlyUndeleted(t *testing.T) {
	resetStoreForTest(t)
	ctx := context.Background()

	if _, _, err := CreatePlayerForUIDContext(ctx, 7001, "HeroA"); err != nil {
		t.Fatalf("create A: %v", err)
	}
	infoB, _, err := CreatePlayerForUIDContext(ctx, 7001, "HeroB")
	if err != nil {
		t.Fatalf("create B: %v", err)
	}
	if err := DeletePlayerContext(ctx, 7001, infoB.PlayerId); err != nil {
		t.Fatalf("delete B: %v", err)
	}

	players, err := ListPlayersByUIDContext(ctx, 7001)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(players) != 1 {
		t.Fatalf("expected 1 undeleted player, got %d", len(players))
	}
	if players[0].Name != "HeroA" {
		t.Fatalf("expected HeroA remains, got %s", players[0].Name)
	}
}

func TestCreatePlayerForUIDRejectsAtLimit(t *testing.T) {
	resetStoreForTest(t)
	setMaxCharactersForTest(t, 2)
	ctx := context.Background()

	if _, _, err := CreatePlayerForUIDContext(ctx, 8001, "One"); err != nil {
		t.Fatalf("create one: %v", err)
	}
	if _, _, err := CreatePlayerForUIDContext(ctx, 8001, "Two"); err != nil {
		t.Fatalf("create two: %v", err)
	}
	if _, _, err := CreatePlayerForUIDContext(ctx, 8001, "Three"); !errors.Is(err, ErrPlayerLimitExceeded) {
		t.Fatalf("expected ErrPlayerLimitExceeded, got %v", err)
	}
}

func TestDeletePlayerRejectsForeignOwner(t *testing.T) {
	resetStoreForTest(t)
	ctx := context.Background()

	info, _, err := CreatePlayerForUIDContext(ctx, 9001, "Owner")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := DeletePlayerContext(ctx, 9002, info.PlayerId); !errors.Is(err, ErrPlayerNotFound) {
		t.Fatalf("expected ErrPlayerNotFound for foreign owner, got %v", err)
	}
}

func TestDeletePlayerAlreadyDeleted(t *testing.T) {
	resetStoreForTest(t)
	ctx := context.Background()

	info, _, err := CreatePlayerForUIDContext(ctx, 9101, "Once")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := DeletePlayerContext(ctx, 9101, info.PlayerId); err != nil {
		t.Fatalf("first delete: %v", err)
	}
	if err := DeletePlayerContext(ctx, 9101, info.PlayerId); !errors.Is(err, ErrPlayerDeleted) {
		t.Fatalf("expected ErrPlayerDeleted, got %v", err)
	}
}

func TestGetPlayerByPlayerIDRejectsDeleted(t *testing.T) {
	resetStoreForTest(t)
	ctx := context.Background()

	info, _, err := CreatePlayerForUIDContext(ctx, 9201, "Ghost")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := DeletePlayerContext(ctx, 9201, info.PlayerId); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, found, err := GetPlayerByPlayerIDContext(ctx, 9201, info.PlayerId)
	if err != nil {
		t.Fatalf("get deleted: %v", err)
	}
	if found {
		t.Fatal("expected deleted player not found")
	}
}

func TestGetPlayerByPlayerIDRejectsForeignOwner(t *testing.T) {
	resetStoreForTest(t)
	ctx := context.Background()

	info, _, err := CreatePlayerForUIDContext(ctx, 9301, "Mine")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, found, err := GetPlayerByPlayerIDContext(ctx, 9302, info.PlayerId)
	if err != nil {
		t.Fatalf("get foreign: %v", err)
	}
	if found {
		t.Fatal("expected foreign player not found")
	}
}
