package persistence

import (
	"errors"
	"testing"
	"time"

	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/persistence/model"
)

func TestGMUserAuthAndBootstrap(t *testing.T) {
	UseMemoryDBForTest(t)
	if err := EnsureGMBootstrap("admin", "admin123"); err != nil {
		t.Fatal(err)
	}
	if err := EnsureGMBootstrap("other", "other12"); err != nil {
		t.Fatal(err)
	}
	var n int64
	if err := db.Model(&model.GMUser{}).Count(&n).Error; err != nil || n != 1 {
		t.Fatalf("bootstrap once: n=%d err=%v", n, err)
	}
	u, err := AuthenticateGMUser("admin", "admin123")
	if err != nil || u.Role != GMRoleAdmin || u.Username != "admin" {
		t.Fatalf("auth: %+v err=%v", u, err)
	}
	if _, err := AuthenticateGMUser("admin", "wrongpwd"); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("bad pass err=%v", err)
	}
}

func TestGMUserDisableLastAdmin(t *testing.T) {
	UseMemoryDBForTest(t)
	admin, err := CreateGMUser("boss", "secret12", "Boss", GMRoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if err := SetGMUserDisabled(admin.ID, true); !errors.Is(err, ErrGMLastAdmin) {
		t.Fatalf("last admin err=%v", err)
	}
	op, err := CreateGMUser("op1", "secret12", "", GMRoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	if err := SetGMUserDisabled(op.ID, true); err != nil {
		t.Fatal(err)
	}
	v, err := AuthenticateGMUser("op1", "secret12")
	if v != nil || !errors.Is(err, ErrGMUserDisabled) {
		t.Fatalf("disabled login: %v %v", v, err)
	}
}

func TestGMSessionMemory(t *testing.T) {
	UseMemoryDBForTest(t)
	tok, err := NewGMSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	if err := PutGMSession(tok, GMSession{UserID: 1, Username: "a", Role: GMRoleAdmin}, time.Hour); err != nil {
		t.Fatal(err)
	}
	s, err := GetGMSession(tok)
	if err != nil || s.Username != "a" {
		t.Fatalf("get %+v err=%v", s, err)
	}
	DeleteGMSession(tok)
	if _, err := GetGMSession(tok); !errors.Is(err, ErrGMSessionGone) {
		t.Fatalf("deleted err=%v", err)
	}
}

func TestListGMOpLogsFilter(t *testing.T) {
	UseMemoryDBForTest(t)
	WriteGMOpLog("alice", GMActionGrant, 1, 2, "item=1", code.OK)
	WriteGMOpLog("bob", GMActionKick, 3, 0, "", code.OK)
	list, total, page, size, err := ListGMOpLogs(GMOpLogFilter{Operator: "alice", Page: 1, PageSize: 10})
	if err != nil || total != 1 || page != 1 || size != 10 || len(list) != 1 || list[0].Operator != "alice" {
		t.Fatalf("filter: list=%+v total=%d err=%v", list, total, err)
	}
}
