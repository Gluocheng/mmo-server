package persistence

import (
	"testing"
	"time"

	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/persistence/model"
)

func seedGMAccountPlayer(t *testing.T, uid int64, nickname string, playerID int64, name string, deleted bool) {
	t.Helper()
	acc := model.Account{UID: uid, Nickname: nickname, Password: "hash"}
	if err := db.Create(&acc).Error; err != nil {
		t.Fatalf("create account: %v", err)
	}
	p := model.Player{PlayerID: playerID, UID: uid, Name: name}
	if deleted {
		now := time.Now()
		p.DeletedAt = &now
	}
	if err := db.Create(&p).Error; err != nil {
		t.Fatalf("create player: %v", err)
	}
}

func TestGetAccountByUIDAndNickname(t *testing.T) {
	resetDBForTest(t)
	seedGMAccountPlayer(t, 11, "alice", 101, "hero", false)

	view, found, err := GetAccountByUID(11)
	if err != nil || !found || view.Nickname != "alice" || view.Uid != 11 {
		t.Fatalf("by uid: view=%+v found=%v err=%v", view, found, err)
	}
	if view.CreatedAtUnix < 1 {
		t.Fatalf("expected createdAtUnix, got %d", view.CreatedAtUnix)
	}
	view2, found, err := GetAccountByNickname("alice")
	if err != nil || !found || view2.Uid != 11 {
		t.Fatalf("by nickname: view=%+v found=%v err=%v", view2, found, err)
	}
	_, found, err = GetAccountByUID(999)
	if err != nil || found {
		t.Fatalf("missing uid: found=%v err=%v", found, err)
	}
}

func TestGetPlayerByIDIncludesDeleted(t *testing.T) {
	resetDBForTest(t)
	seedGMAccountPlayer(t, 12, "bob", 201, "gone", true)

	rec, found, err := GetPlayerByID(201)
	if err != nil || !found {
		t.Fatalf("get: found=%v err=%v", found, err)
	}
	if !rec.Deleted || rec.Uid != 12 || rec.Name != "gone" {
		t.Fatalf("unexpected record %+v", rec)
	}
}

func TestListPlayersByUIDAllAndByName(t *testing.T) {
	resetDBForTest(t)
	seedGMAccountPlayer(t, 13, "carol", 301, "alive", false)
	p := model.Player{PlayerID: 302, UID: 13, Name: "dead"}
	now := time.Now()
	p.DeletedAt = &now
	if err := db.Create(&p).Error; err != nil {
		t.Fatalf("create deleted: %v", err)
	}

	list, err := ListPlayersByUIDAll(13)
	if err != nil || len(list) != 2 {
		t.Fatalf("list uid: n=%d err=%v", len(list), err)
	}
	named, err := GetPlayersByName("alive")
	if err != nil || len(named) != 1 || named[0].PlayerId != 301 {
		t.Fatalf("by name: %+v err=%v", named, err)
	}
}

func TestWriteGMOpLog(t *testing.T) {
	resetDBForTest(t)
	WriteGMOpLog("op1", GMActionGrant, 13, 301, "item=10", code.OK)
	var n int64
	if err := db.Model(&model.GMOpLog{}).Count(&n).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 log, got %d", n)
	}
	var row model.GMOpLog
	if err := db.First(&row).Error; err != nil {
		t.Fatalf("read: %v", err)
	}
	if row.Operator != "op1" || row.Action != GMActionGrant || row.TargetPlayerID != 301 {
		t.Fatalf("unexpected row %+v", row)
	}
}
