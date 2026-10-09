package persistence

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/example/mmo-server/internal/code"
	"github.com/redis/go-redis/v9"
)

func newPartyTest(t *testing.T) context.Context {
	t.Helper()
	UseMemoryDBForTest(t)
	UseRedisForTest(t)
	return context.Background()
}

func putPartyOnline(t *testing.T, ctx context.Context, uid int64, node string) {
	t.Helper()
	key := fmt.Sprintf("%s:party:online:%d", KeyPrefix(), uid)
	if err := rdb.Set(ctx, key, node, 0).Err(); err != nil {
		t.Fatalf("set online %d: %v", uid, err)
	}
}

func partyOnlineValue(t *testing.T, ctx context.Context, uid int64) (string, bool) {
	t.Helper()
	key := fmt.Sprintf("%s:party:online:%d", KeyPrefix(), uid)
	v, err := rdb.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", false
	}
	if err != nil {
		t.Fatalf("get online %d: %v", uid, err)
	}
	return v, true
}

func wantCode(t *testing.T, got, want int32) {
	t.Helper()
	if got != want {
		t.Fatalf("code=%d want %d", got, want)
	}
}

func wantMembers(t *testing.T, st PartyState, uids ...int64) {
	t.Helper()
	if len(st.Members) != len(uids) {
		t.Fatalf("members=%v want %v", st.Members, uids)
	}
	for i, uid := range uids {
		if st.Members[i] != uid {
			t.Fatalf("members=%v want %v", st.Members, uids)
		}
	}
}

func wantNoCode(t *testing.T, got int32) {
	t.Helper()
	if got == code.PlayerNotEntered {
		t.Fatalf("storage returned 40009")
	}
}

// TestPartyStorageDoesNotReturnNotEntered 存储层不能表达未进场：无在线节点是 40065。
func TestPartyStorageDoesNotReturnNotEntered(t *testing.T) {
	ctx := newPartyTest(t)
	const now int64 = 1_700_000_000_000

	st, c := CreateParty(ctx, 1, 10, "10001", now)
	wantNoCode(t, c)
	wantCode(t, c, code.OK)
	if st.PartyID != 10 || st.LeaderUID != 1 {
		t.Fatalf("create state=%+v", st)
	}
	wantMembers(t, st, 1)
	node, ok := partyOnlineValue(t, ctx, 1)
	if !ok || node != "10001" {
		t.Fatalf("online=%q ok=%v", node, ok)
	}

	c = Invite(ctx, 1, 2, 900, now+30_000, now)
	wantNoCode(t, c)
	wantCode(t, c, code.PartyTargetOffline)
}

func TestPartyCreateTwice(t *testing.T) {
	ctx := newPartyTest(t)
	const now int64 = 1_700_000_000_000

	if _, c := CreateParty(ctx, 1, 10, "10001", now); c != code.OK {
		t.Fatalf("create %d", c)
	}
	_, c := CreateParty(ctx, 1, 11, "10001", now+1)
	wantCode(t, c, code.PartyAlreadyIn)
	st, c := PartyStateOf(ctx, 1, now+1)
	wantCode(t, c, code.OK)
	if st.PartyID != 10 || st.LeaderUID != 1 {
		t.Fatalf("still original: %+v", st)
	}

	if err := BeginGrace(ctx, 1, now+1); err != nil {
		t.Fatal(err)
	}
	_, c = CreateParty(ctx, 1, 12, "10001", now+2)
	wantCode(t, c, code.PartyAlreadyIn)

	st, c = CreateParty(ctx, 1, 13, "10002", now+60_001+1)
	wantCode(t, c, code.OK)
	if st.PartyID != 13 || st.LeaderUID != 1 {
		t.Fatalf("recreate after grace: %+v", st)
	}
	wantMembers(t, st, 1)
}

func TestPartyInviteRules(t *testing.T) {
	ctx := newPartyTest(t)
	const now int64 = 1_700_000_000_000

	if _, c := CreateParty(ctx, 1, 10, "10001", now); c != code.OK {
		t.Fatalf("create %d", c)
	}
	putPartyOnline(t, ctx, 2, "10001")
	putPartyOnline(t, ctx, 3, "10001")

	if c := Invite(ctx, 1, 1, 1, now+30_000, now); c != code.PartyInviteInvalid {
		t.Fatalf("invite self %d", c)
	}
	if c := Invite(ctx, 1, 2, 2, now+30_000, now); c != code.OK {
		t.Fatalf("invite %d", c)
	}
	if _, c := Answer(ctx, 2, 2, true, now); c != code.OK {
		t.Fatalf("answer %d", c)
	}

	putPartyOnline(t, ctx, 4, "10001")
	if c := Invite(ctx, 2, 4, 3, now+30_000, now); c != code.PartyNotLeader {
		t.Fatalf("member invite %d", c)
	}

	if _, c := CreateParty(ctx, 3, 20, "10001", now); c != code.OK {
		t.Fatalf("create other %d", c)
	}
	if c := Invite(ctx, 1, 3, 4, now+30_000, now); c != code.PartyAlreadyIn {
		t.Fatalf("target in party %d", c)
	}

	if err := BeginGrace(ctx, 2, now); err != nil {
		t.Fatal(err)
	}
	if _, ok := partyOnlineValue(t, ctx, 2); ok {
		t.Fatal("grace member still online")
	}
	if c := Invite(ctx, 1, 2, 5, now+1_000, now+1); c != code.PartyAlreadyIn {
		t.Fatalf("grace target %d want 40060", c)
	}
}

func TestPartyAnswer(t *testing.T) {
	const now int64 = 1_700_000_000_000

	t.Run("accept appends in join order", func(t *testing.T) {
		ctx := newPartyTest(t)
		if _, c := CreateParty(ctx, 1, 10, "10001", now); c != code.OK {
			t.Fatal(c)
		}
		putPartyOnline(t, ctx, 2, "10001")
		putPartyOnline(t, ctx, 3, "10001")
		if c := Invite(ctx, 1, 2, 21, now+30_000, now); c != code.OK {
			t.Fatal(c)
		}
		st, c := Answer(ctx, 2, 21, true, now)
		wantCode(t, c, code.OK)
		wantMembers(t, st, 1, 2)
		if c := Invite(ctx, 1, 3, 22, now+30_000, now); c != code.OK {
			t.Fatal(c)
		}
		st, c = Answer(ctx, 3, 22, true, now)
		wantCode(t, c, code.OK)
		wantMembers(t, st, 1, 2, 3)
		if st.LeaderUID != 1 || st.PartyID != 10 {
			t.Fatalf("state=%+v", st)
		}
	})

	t.Run("reject leaves members", func(t *testing.T) {
		ctx := newPartyTest(t)
		if _, c := CreateParty(ctx, 1, 10, "10001", now); c != code.OK {
			t.Fatal(c)
		}
		putPartyOnline(t, ctx, 2, "10001")
		if c := Invite(ctx, 1, 2, 21, now+30_000, now); c != code.OK {
			t.Fatal(c)
		}
		st, c := Answer(ctx, 2, 21, false, now)
		wantCode(t, c, code.PartyInviteInvalid)
		wantMembers(t, st, 1)
		got, c := PartyStateOf(ctx, 1, now)
		wantCode(t, c, code.OK)
		wantMembers(t, got, 1)
		_, c = Answer(ctx, 2, 21, true, now)
		wantCode(t, c, code.PartyInviteInvalid)
	})

	t.Run("now past expireAt rejects while key ttl remains", func(t *testing.T) {
		ctx := newPartyTest(t)
		if _, c := CreateParty(ctx, 1, 10, "10001", now); c != code.OK {
			t.Fatal(c)
		}
		putPartyOnline(t, ctx, 2, "10001")
		expireAt := now + 1_000
		if c := Invite(ctx, 1, 2, 21, expireAt, now); c != code.OK {
			t.Fatal(c)
		}
		key := fmt.Sprintf("%s:party:invite:%d", KeyPrefix(), 2)
		ttl, err := rdb.TTL(ctx, key).Result()
		if err != nil {
			t.Fatal(err)
		}
		if ttl <= 0 || ttl > 30*time.Second {
			t.Fatalf("invite ttl=%s", ttl)
		}
		if _, err := rdb.Get(ctx, key).Result(); err != nil {
			t.Fatalf("invite key missing before logical expiry: %v", err)
		}
		st, c := Answer(ctx, 2, 21, true, expireAt)
		wantCode(t, c, code.OK)
		wantMembers(t, st, 1, 2)

		if _, c := CreateParty(ctx, 3, 11, "10001", now); c != code.OK {
			t.Fatal(c)
		}
		putPartyOnline(t, ctx, 4, "10001")
		if c := Invite(ctx, 3, 4, 31, now+1_000, now); c != code.OK {
			t.Fatal(c)
		}
		st, c = Answer(ctx, 4, 31, true, now+1_001)
		wantCode(t, c, code.PartyInviteInvalid)
		wantMembers(t, st, 3)
		got, _ := PartyStateOf(ctx, 3, now+1_001)
		wantMembers(t, got, 3)
	})

	t.Run("new invite invalidates old id", func(t *testing.T) {
		ctx := newPartyTest(t)
		if _, c := CreateParty(ctx, 1, 10, "10001", now); c != code.OK {
			t.Fatal(c)
		}
		putPartyOnline(t, ctx, 2, "10001")
		if c := Invite(ctx, 1, 2, 21, now+30_000, now); c != code.OK {
			t.Fatal(c)
		}
		if c := Invite(ctx, 1, 2, 22, now+30_000, now+1); c != code.OK {
			t.Fatal(c)
		}
		if _, c := Answer(ctx, 2, 21, true, now+1); c != code.PartyInviteInvalid {
			t.Fatalf("old id %d", c)
		}
		st, c := Answer(ctx, 2, 22, true, now+1)
		wantCode(t, c, code.OK)
		wantMembers(t, st, 1, 2)
	})
}

func TestPartyFull(t *testing.T) {
	ctx := newPartyTest(t)
	const now int64 = 1_700_000_000_000

	if _, c := CreateParty(ctx, 1, 10, "10001", now); c != code.OK {
		t.Fatal(c)
	}
	for i, uid := range []int64{2, 3, 4} {
		putPartyOnline(t, ctx, uid, "10001")
		inv := int64(100 + i)
		if c := Invite(ctx, 1, uid, inv, now+30_000, now); c != code.OK {
			t.Fatal(c)
		}
		st, c := Answer(ctx, uid, inv, true, now)
		wantCode(t, c, code.OK)
		if len(st.Members) != i+2 {
			t.Fatalf("len=%d", len(st.Members))
		}
	}
	if err := BeginGrace(ctx, 4, now); err != nil {
		t.Fatal(err)
	}
	st, c := PartyStateOf(ctx, 1, now)
	wantCode(t, c, code.OK)
	wantMembers(t, st, 1, 2, 3, 4)

	putPartyOnline(t, ctx, 5, "10001")
	if c := Invite(ctx, 1, 5, 200, now+30_000, now); c != code.OK {
		t.Fatal(c)
	}
	st, c = Answer(ctx, 5, 200, true, now)
	wantCode(t, c, code.PartyFull)
	wantMembers(t, st, 1, 2, 3, 4)
	got, _ := PartyStateOf(ctx, 1, now)
	wantMembers(t, got, 1, 2, 3, 4)
	empty, c := PartyStateOf(ctx, 5, now)
	wantCode(t, c, code.OK)
	if empty.PartyID != 0 || len(empty.Members) != 0 {
		t.Fatalf("fifth joined: %+v", empty)
	}
	if _, c := Answer(ctx, 5, 200, true, now); c != code.PartyInviteInvalid {
		t.Fatalf("full invite still live %d", c)
	}
}

func TestPartyLeave(t *testing.T) {
	const now int64 = 1_700_000_000_000

	t.Run("leader hands off to earliest remaining", func(t *testing.T) {
		ctx := newPartyTest(t)
		mustPartyOf(t, ctx, []int64{1, 2, 3}, 10, now)
		left, rest, c := Leave(ctx, 1, now)
		wantCode(t, c, code.OK)
		if left.PartyID != 0 || len(left.Members) != 0 {
			t.Fatalf("left=%+v", left)
		}
		if rest.PartyID != 10 || rest.LeaderUID != 2 {
			t.Fatalf("rest=%+v", rest)
		}
		wantMembers(t, rest, 2, 3)
		gone, _ := PartyStateOf(ctx, 1, now)
		if gone.PartyID != 0 {
			t.Fatalf("leaver still in %+v", gone)
		}
		got, _ := PartyStateOf(ctx, 2, now)
		if got.LeaderUID != 2 {
			t.Fatalf("successor %+v", got)
		}
		wantMembers(t, got, 2, 3)
	})

	t.Run("solo leave deletes party", func(t *testing.T) {
		ctx := newPartyTest(t)
		if _, c := CreateParty(ctx, 1, 10, "10001", now); c != code.OK {
			t.Fatal(c)
		}
		left, rest, c := Leave(ctx, 1, now)
		wantCode(t, c, code.OK)
		if left.PartyID != 0 || rest.PartyID != 0 {
			t.Fatalf("left=%+v rest=%+v", left, rest)
		}
		if _, err := rdb.Get(ctx, fmt.Sprintf("%s:party:%d", KeyPrefix(), 10)).Result(); !errors.Is(err, redis.Nil) {
			t.Fatalf("party key err=%v", err)
		}
		st, _ := PartyStateOf(ctx, 1, now)
		if st.PartyID != 0 {
			t.Fatalf("state=%+v", st)
		}
	})

	t.Run("two members leave dissolves", func(t *testing.T) {
		ctx := newPartyTest(t)
		mustPartyOf(t, ctx, []int64{1, 2}, 10, now)
		_, rest, c := Leave(ctx, 1, now)
		wantCode(t, c, code.OK)
		if rest.PartyID != 0 {
			t.Fatalf("rest=%+v", rest)
		}
		for _, uid := range []int64{1, 2} {
			st, _ := PartyStateOf(ctx, uid, now)
			if st.PartyID != 0 {
				t.Fatalf("uid %d still in %+v", uid, st)
			}
		}
	})

	t.Run("not in party", func(t *testing.T) {
		ctx := newPartyTest(t)
		_, _, c := Leave(ctx, 9, now)
		wantCode(t, c, code.PartyNotIn)
	})
}

func TestPartyKick(t *testing.T) {
	const now int64 = 1_700_000_000_000

	t.Run("kick self", func(t *testing.T) {
		ctx := newPartyTest(t)
		mustPartyOf(t, ctx, []int64{1, 2, 3}, 10, now)
		_, _, c := Kick(ctx, 1, 1, now)
		wantCode(t, c, code.PartyInviteInvalid)
		got, _ := PartyStateOf(ctx, 1, now)
		wantMembers(t, got, 1, 2, 3)
	})

	t.Run("kick member", func(t *testing.T) {
		ctx := newPartyTest(t)
		mustPartyOf(t, ctx, []int64{1, 2, 3}, 10, now)
		left, rest, c := Kick(ctx, 1, 3, now)
		wantCode(t, c, code.OK)
		if left.PartyID != 0 {
			t.Fatalf("left=%+v", left)
		}
		wantMembers(t, rest, 1, 2)
		if rest.LeaderUID != 1 || rest.PartyID != 10 {
			t.Fatalf("rest=%+v", rest)
		}
		gone, c := PartyStateOf(ctx, 3, now)
		wantCode(t, c, code.OK)
		if gone.PartyID != 0 || len(gone.Members) != 0 {
			t.Fatalf("kicked=%+v", gone)
		}
	})

	t.Run("kick grace member clears seat", func(t *testing.T) {
		ctx := newPartyTest(t)
		mustPartyOf(t, ctx, []int64{1, 2, 3}, 10, now)
		if err := BeginGrace(ctx, 2, now); err != nil {
			t.Fatal(err)
		}
		_, rest, c := Kick(ctx, 1, 2, now+1)
		wantCode(t, c, code.OK)
		wantMembers(t, rest, 1, 3)
		gone, _ := PartyStateOf(ctx, 2, now+1)
		if gone.PartyID != 0 {
			t.Fatalf("grace kicked still in %+v", gone)
		}
		raw, err := rdb.Get(ctx, fmt.Sprintf("%s:party:%d", KeyPrefix(), 10)).Result()
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(raw, strconv.FormatInt(now+60_000, 10)) {
			t.Fatalf("grace still stored: %s", raw)
		}
	})

	t.Run("member cannot kick", func(t *testing.T) {
		ctx := newPartyTest(t)
		mustPartyOf(t, ctx, []int64{1, 2, 3}, 10, now)
		_, _, c := Kick(ctx, 2, 3, now)
		wantCode(t, c, code.PartyNotLeader)
	})
}

func TestPartyGrace(t *testing.T) {
	const now int64 = 1_700_000_000_000

	t.Run("begin grace keeps seat and drops online", func(t *testing.T) {
		ctx := newPartyTest(t)
		mustPartyOf(t, ctx, []int64{1, 2, 3}, 10, now)
		if err := BeginGrace(ctx, 2, now); err != nil {
			t.Fatal(err)
		}
		if _, ok := partyOnlineValue(t, ctx, 2); ok {
			t.Fatal("online key remains")
		}
		raw, err := rdb.Get(ctx, fmt.Sprintf("%s:party:%d", KeyPrefix(), 10)).Result()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(raw, strconv.FormatInt(now+60_000, 10)) {
			t.Fatalf("graceUntil want %d in %s", now+60_000, raw)
		}
		st, c := PartyStateOf(ctx, 1, now+60_000)
		wantCode(t, c, code.OK)
		wantMembers(t, st, 1, 2, 3)
		if st.Members[1] != 2 {
			t.Fatalf("index=%v", st.Members)
		}
	})

	t.Run("on enter within grace restores index", func(t *testing.T) {
		ctx := newPartyTest(t)
		mustPartyOf(t, ctx, []int64{1, 2, 3}, 10, now)
		if err := BeginGrace(ctx, 2, now); err != nil {
			t.Fatal(err)
		}
		st, back, err := OnEnter(ctx, 2, "10009", now+1_000)
		if err != nil {
			t.Fatal(err)
		}
		if !back {
			t.Fatal("expected rejoin")
		}
		wantMembers(t, st, 1, 2, 3)
		if st.Members[1] != 2 {
			t.Fatalf("index=%v", st.Members)
		}
		node, ok := partyOnlineValue(t, ctx, 2)
		if !ok || node != "10009" {
			t.Fatalf("online=%q ok=%v", node, ok)
		}
		raw, err := rdb.Get(ctx, fmt.Sprintf("%s:party:%d", KeyPrefix(), 10)).Result()
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(raw, strconv.FormatInt(now+60_000, 10)) {
			t.Fatalf("grace not cleared: %s", raw)
		}
		later, _ := PartyStateOf(ctx, 1, now+60_001)
		wantMembers(t, later, 1, 2, 3)
	})

	t.Run("grace expiry removes member", func(t *testing.T) {
		ctx := newPartyTest(t)
		mustPartyOf(t, ctx, []int64{1, 2, 3}, 10, now)
		if err := BeginGrace(ctx, 2, now); err != nil {
			t.Fatal(err)
		}
		st, c := PartyStateOf(ctx, 1, now+60_001)
		wantCode(t, c, code.OK)
		wantMembers(t, st, 1, 3)
		if st.LeaderUID != 1 {
			t.Fatalf("leader=%d", st.LeaderUID)
		}
		gone, _ := PartyStateOf(ctx, 2, now+60_001)
		if gone.PartyID != 0 {
			t.Fatalf("expired still in %+v", gone)
		}
	})

	t.Run("on enter after grace does not rejoin", func(t *testing.T) {
		ctx := newPartyTest(t)
		mustPartyOf(t, ctx, []int64{1, 2, 3}, 10, now)
		if err := BeginGrace(ctx, 2, now); err != nil {
			t.Fatal(err)
		}
		st, back, err := OnEnter(ctx, 2, "10009", now+60_001)
		if err != nil {
			t.Fatal(err)
		}
		if back || st.PartyID != 0 {
			t.Fatalf("back=%v state=%+v", back, st)
		}
		got, _ := PartyStateOf(ctx, 1, now+60_001)
		wantMembers(t, got, 1, 3)
		node, ok := partyOnlineValue(t, ctx, 2)
		if !ok || node != "10009" {
			t.Fatalf("enter should write online, got %q ok=%v", node, ok)
		}
	})

	t.Run("leader grace expiry promotes earliest", func(t *testing.T) {
		ctx := newPartyTest(t)
		mustPartyOf(t, ctx, []int64{1, 2, 3}, 10, now)
		if err := BeginGrace(ctx, 1, now); err != nil {
			t.Fatal(err)
		}
		st, back, err := OnEnter(ctx, 1, "10001", now+60_001)
		if err != nil {
			t.Fatal(err)
		}
		if back || st.PartyID != 0 {
			t.Fatalf("leader rejoined %+v back=%v", st, back)
		}
		got, _ := PartyStateOf(ctx, 2, now+60_001)
		if got.PartyID != 10 || got.LeaderUID != 2 {
			t.Fatalf("successor %+v", got)
		}
		wantMembers(t, got, 2, 3)
	})

	t.Run("grace expiry below two dissolves", func(t *testing.T) {
		ctx := newPartyTest(t)
		mustPartyOf(t, ctx, []int64{1, 2}, 10, now)
		if err := BeginGrace(ctx, 2, now); err != nil {
			t.Fatal(err)
		}
		st, _ := PartyStateOf(ctx, 1, now+60_001)
		if st.PartyID != 0 {
			t.Fatalf("party survived %+v", st)
		}
		gone, _ := PartyStateOf(ctx, 2, now+60_001)
		if gone.PartyID != 0 {
			t.Fatalf("member survived %+v", gone)
		}
		if _, err := rdb.Get(ctx, fmt.Sprintf("%s:party:%d", KeyPrefix(), 10)).Result(); !errors.Is(err, redis.Nil) {
			t.Fatalf("party key err=%v", err)
		}
	})
}

func TestPartyOnEnterWithoutGrace(t *testing.T) {
	ctx := newPartyTest(t)
	const now int64 = 1_700_000_000_000
	mustPartyOf(t, ctx, []int64{1, 2, 3}, 10, now)
	if err := rdb.Del(ctx, fmt.Sprintf("%s:party:online:%d", KeyPrefix(), 2)).Err(); err != nil {
		t.Fatal(err)
	}
	st, back, err := OnEnter(ctx, 2, "10008", now+86_400_000)
	if err != nil {
		t.Fatal(err)
	}
	if !back {
		t.Fatal("crash rejoin")
	}
	wantMembers(t, st, 1, 2, 3)
	if st.Members[1] != 2 {
		t.Fatalf("index=%v", st.Members)
	}
	node, ok := partyOnlineValue(t, ctx, 2)
	if !ok || node != "10008" {
		t.Fatalf("online=%q ok=%v", node, ok)
	}
}

func TestPartyCreateAfterGraceKeepsSurvivors(t *testing.T) {
	const now int64 = 1_700_000_000_000

	t.Run("new party id", func(t *testing.T) {
		ctx := newPartyTest(t)
		mustPartyOf(t, ctx, []int64{1, 2, 3}, 10, now)
		if err := BeginGrace(ctx, 3, now); err != nil {
			t.Fatal(err)
		}
		st, c := CreateParty(ctx, 3, 11, "10002", now+60_001)
		wantCode(t, c, code.OK)
		if st.PartyID != 11 || st.LeaderUID != 3 {
			t.Fatalf("created=%+v", st)
		}
		wantMembers(t, st, 3)
		old, c := PartyStateOf(ctx, 1, now+60_001)
		wantCode(t, c, code.OK)
		if old.PartyID != 10 || old.LeaderUID != 1 {
			t.Fatalf("survivors=%+v", old)
		}
		wantMembers(t, old, 1, 2)
	})

	t.Run("live party id is not replaced", func(t *testing.T) {
		ctx := newPartyTest(t)
		mustPartyOf(t, ctx, []int64{1, 2, 3}, 10, now)
		if err := BeginGrace(ctx, 3, now); err != nil {
			t.Fatal(err)
		}
		if _, c := CreateParty(ctx, 3, 10, "10002", now+60_001); c == code.OK {
			t.Fatal("reused a live party id")
		}
		old, _ := PartyStateOf(ctx, 1, now+60_001)
		if old.PartyID != 10 || old.LeaderUID != 1 {
			t.Fatalf("survivors=%+v", old)
		}
		wantMembers(t, old, 1, 2)
		self, _ := PartyStateOf(ctx, 3, now+60_001)
		if self.PartyID != 0 {
			t.Fatalf("expired member replaced roster: %+v", self)
		}
	})
}

func TestPartyCrossNodeRoster(t *testing.T) {
	ctx := newPartyTest(t)
	const now int64 = 1_700_000_000_000

	if _, c := CreateParty(ctx, 1, 10, "10001", now); c != code.OK {
		t.Fatal(c)
	}
	putPartyOnline(t, ctx, 2, "10002")
	if c := Invite(ctx, 1, 2, 21, now+30_000, now); c != code.OK {
		t.Fatalf("invite %d", c)
	}
	if _, c := Answer(ctx, 2, 21, true, now); c != code.OK {
		t.Fatal(c)
	}
	a, c := PartyStateOf(ctx, 1, now)
	wantCode(t, c, code.OK)
	b, c := PartyStateOf(ctx, 2, now)
	wantCode(t, c, code.OK)
	if a.PartyID != b.PartyID || a.LeaderUID != b.LeaderUID {
		t.Fatalf("a=%+v b=%+v", a, b)
	}
	wantMembers(t, a, 1, 2)
	wantMembers(t, b, 1, 2)

	if err := BeginGrace(ctx, 2, now); err != nil {
		t.Fatal(err)
	}
	a, _ = PartyStateOf(ctx, 1, now+60_001)
	b, _ = PartyStateOf(ctx, 2, now+60_001)
	if containsUID(a.Members, 2) || containsUID(b.Members, 2) {
		t.Fatalf("still listed a=%v b=%v", a.Members, b.Members)
	}
}

func mustPartyOf(t *testing.T, ctx context.Context, uids []int64, partyID, now int64) {
	t.Helper()
	if len(uids) < 1 {
		t.Fatal("empty")
	}
	if _, c := CreateParty(ctx, uids[0], partyID, "10001", now); c != code.OK {
		t.Fatalf("create %d", c)
	}
	for i, uid := range uids[1:] {
		putPartyOnline(t, ctx, uid, "10001")
		inv := partyID*100 + int64(i) + 1
		if c := Invite(ctx, uids[0], uid, inv, now+30_000, now); c != code.OK {
			t.Fatalf("invite %d: %d", uid, c)
		}
		if _, c := Answer(ctx, uid, inv, true, now); c != code.OK {
			t.Fatalf("answer %d: %d", uid, c)
		}
	}
}

func containsUID(members []int64, uid int64) bool {
	for _, id := range members {
		if id == uid {
			return true
		}
	}
	return false
}
