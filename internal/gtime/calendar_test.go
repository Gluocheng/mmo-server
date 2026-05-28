package gtime

import (
	"testing"
	"time"
)

func TestIsOverDayWithResetPoint(t *testing.T) {
	loc := time.Local
	secPnt := 5 * 3600
	// 05:00 刷新：06:00 与 08:00 属同一游戏日
	t1 := time.Date(2024, 6, 1, 6, 0, 0, 0, loc).Unix()
	t2 := time.Date(2024, 6, 1, 8, 0, 0, 0, loc).Unix()
	if IsOverDay(t1, t2, secPnt) {
		t.Fatal("expected same game day")
	}
	// 04:00 仍属上一游戏日，06:00 起为新游戏日
	before := time.Date(2024, 6, 1, 4, 0, 0, 0, loc).Unix()
	after := time.Date(2024, 6, 1, 6, 0, 0, 0, loc).Unix()
	if !IsOverDay(before, after, secPnt) {
		t.Fatal("expected cross game day across 05:00 reset")
	}
}

func TestInSameCalendarDay(t *testing.T) {
	loc := time.Local
	t1 := time.Date(2024, 1, 1, 1, 0, 0, 0, loc).Unix()
	t2 := time.Date(2024, 1, 1, 23, 0, 0, 0, loc).Unix()
	if !InSameCalendarDay(t1, t2) {
		t.Fatal("expected same calendar day")
	}
}

func TestIsInDayTimeRange(t *testing.T) {
	loc := time.Local
	now := time.Date(2024, 3, 10, 12, 30, 0, 0, loc).Unix()
	if !IsInDayTimeRange(now, 12*3600, 13*3600) {
		t.Fatal("expected in range")
	}
	if IsInDayTimeRange(now, 13*3600, 14*3600) {
		t.Fatal("expected out of range")
	}
}
