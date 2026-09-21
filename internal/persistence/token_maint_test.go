package persistence

import (
	"context"
	"errors"
	"testing"

	"github.com/redis/go-redis/v9"
)

func TestRevokeAllTokensForUID(t *testing.T) {
	UseMemoryDBForTest(t)
	UseRedisForTest(t)
	ctx := context.Background()
	access, _, refresh, _, err := issueTokenPairWithUID(ctx, 88, "dev-1")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if err := rdb.Get(ctx, accessTokenKey(access)).Err(); err != nil {
		t.Fatalf("access missing: %v", err)
	}
	if err := rdb.Get(ctx, refreshTokenKey(refresh)).Err(); err != nil {
		t.Fatalf("refresh missing: %v", err)
	}
	if err := RevokeAllTokensForUID(88); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if err := rdb.Get(ctx, accessTokenKey(access)).Err(); !errors.Is(err, redis.Nil) {
		t.Fatalf("expected access gone, err=%v", err)
	}
	if err := rdb.Get(ctx, refreshTokenKey(refresh)).Err(); !errors.Is(err, redis.Nil) {
		t.Fatalf("expected refresh gone, err=%v", err)
	}
}

func TestMaintenanceFlag(t *testing.T) {
	UseMemoryDBForTest(t)
	UseRedisForTest(t)
	ctx := context.Background()
	on, err := IsMaintenance(ctx)
	if err != nil || on {
		t.Fatalf("default: on=%v err=%v", on, err)
	}
	if err := SaveMaintenance(ctx, true, "patch"); err != nil {
		t.Fatal(err)
	}
	st, err := GetMaintenance(ctx)
	if err != nil || !st.Enabled || st.Reason != "patch" {
		t.Fatalf("get: %+v err=%v", st, err)
	}
	if err := SaveMaintenance(ctx, false, "ignored"); err != nil {
		t.Fatal(err)
	}
	st, err = GetMaintenance(ctx)
	if err != nil || st.Enabled || st.Reason != "" {
		t.Fatalf("off: %+v err=%v", st, err)
	}
}
