package gtime

import (
	"math"
	"time"
)

const secondsPerDay = 86400

// ZeroHourUnix 指定 Unix 秒所在日的本地 0 点。
func ZeroHourUnix(t int64) int64 {
	loc := Now().Location()
	d := time.Unix(t, 0).In(loc)
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, loc).Unix()
}

// WeekZeroHourUnix 指定 Unix 秒所在周的周一 0 点（周日算作第 7 天）。
func WeekZeroHourUnix(t int64) int64 {
	loc := Now().Location()
	d := time.Unix(t, 0).In(loc)
	weekday := int32(d.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	t -= (int64(weekday) - 1) * secondsPerDay
	d = time.Unix(t, 0).In(loc)
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, loc).Unix()
}

// MonthZeroHourUnix 指定 Unix 秒所在月的 1 日 0 点。
func MonthZeroHourUnix(t int64) int64 {
	loc := Now().Location()
	d := time.Unix(t, 0).In(loc)
	return time.Date(d.Year(), d.Month(), 1, 0, 0, 0, 0, loc).Unix()
}

// YearZeroHourUnix 指定 Unix 秒所在年的 1 月 1 日 0 点。
func YearZeroHourUnix(t int64) int64 {
	loc := Now().Location()
	d := time.Unix(t, 0).In(loc)
	return time.Date(d.Year(), time.January, 1, 0, 0, 0, 0, loc).Unix()
}

// IsOverDay 是否跨天；secPnt 为日内相对秒数（如 5 点刷新则 secPnt=5*3600）。
func IsOverDay(lastTime, nowTime int64, secPnt int) bool {
	if lastTime > nowTime || secPnt < 0 || secPnt > secondsPerDay {
		return false
	}
	lastZero := ZeroHourUnix(lastTime - int64(secPnt))
	nowZero := ZeroHourUnix(nowTime - int64(secPnt))
	return lastZero < nowZero
}

// IsSameDay 是否同一天（受 secPnt 影响的“游戏日”）。
func IsSameDay(time1, time2 int64, secPnt int) bool {
	if time1 > time2 {
		time1, time2 = time2, time1
	}
	return !IsOverDay(time1, time2, secPnt)
}

// DiffDay 相差游戏日天数。
func DiffDay(lastTime, nowTime int64, secPnt int32) int32 {
	if lastTime >= nowTime || secPnt < 0 || secPnt > secondsPerDay {
		return 0
	}
	lastZero := ZeroHourUnix(lastTime - int64(secPnt))
	nowZero := ZeroHourUnix(nowTime - int64(secPnt))
	return int32((nowZero - lastZero) / secondsPerDay)
}

// IsOverWeek 是否跨周；weekDayPnt：1=周一 … 7=周日；daySecPnt 为当日相对秒数。
func IsOverWeek(lastTime, nowTime int64, weekDayPnt, daySecPnt int) bool {
	if lastTime > nowTime || daySecPnt < 0 || daySecPnt > secondsPerDay || weekDayPnt <= 0 {
		return false
	}
	weekDayPnt = (weekDayPnt - 1) % 7
	lastZero := ZeroHourUnix(lastTime - int64(weekDayPnt*secondsPerDay) - int64(daySecPnt))
	nowZero := ZeroHourUnix(nowTime - int64(weekDayPnt*secondsPerDay) - int64(daySecPnt))
	loc := Now().Location()
	ly, lw := time.Unix(lastZero, 0).In(loc).ISOWeek()
	ny, nw := time.Unix(nowZero, 0).In(loc).ISOWeek()
	return ly != ny || lw != nw
}

// IsOverWeekByWeekSec 是否跨周；weekSecPnt 为一周内相对秒数。
func IsOverWeekByWeekSec(lastTime, nowTime int64, weekSecPnt int) bool {
	weekDayPnt := int(math.Ceil(float64(weekSecPnt+1) / secondsPerDay))
	daySecPnt := weekSecPnt % secondsPerDay
	return IsOverWeek(lastTime, nowTime, weekDayPnt, daySecPnt)
}

// IsOverMonth 是否跨月；dayNumPnt：1=每月 1 日 …；daySecPnt 为当日相对秒数。
func IsOverMonth(lastTime, nowTime int64, dayNumPnt, daySecPnt int) bool {
	if lastTime > nowTime || daySecPnt < 0 || daySecPnt > secondsPerDay || dayNumPnt <= 0 {
		return false
	}
	dayNumPnt--
	lastZero := ZeroHourUnix(lastTime - int64(dayNumPnt*secondsPerDay) - int64(daySecPnt))
	nowZero := ZeroHourUnix(nowTime - int64(dayNumPnt*secondsPerDay) - int64(daySecPnt))
	loc := Now().Location()
	lt := time.Unix(lastZero, 0).In(loc)
	nt := time.Unix(nowZero, 0).In(loc)
	return lt.Year() != nt.Year() || lt.Month() != nt.Month()
}

// IsOverMonthByMonthSec 是否跨月；monthSecPnt 为一月内相对秒数。
func IsOverMonthByMonthSec(lastTime, nowTime int64, monthSecPnt int) bool {
	dayNumPnt := int(math.Ceil(float64(monthSecPnt+1) / secondsPerDay))
	daySecPnt := monthSecPnt % secondsPerDay
	return IsOverMonth(lastTime, nowTime, dayNumPnt, daySecPnt)
}

// DiffMonth 相差月数。
func DiffMonth(lastTime, nowTime int64, dayNumPnt, daySecPnt int) int32 {
	if lastTime > nowTime || daySecPnt < 0 || daySecPnt > secondsPerDay || dayNumPnt <= 0 {
		return 0
	}
	dayNumPnt--
	lastZero := ZeroHourUnix(lastTime - int64(dayNumPnt*secondsPerDay) - int64(daySecPnt))
	nowZero := ZeroHourUnix(nowTime - int64(dayNumPnt*secondsPerDay) - int64(daySecPnt))
	loc := Now().Location()
	t1 := time.Unix(lastZero, 0).In(loc)
	t2 := time.Unix(nowZero, 0).In(loc)
	return int32((t2.Year()-t1.Year())*12 + int(t2.Month()-t1.Month()))
}

// IsInDayTimeRange 当前时刻是否落在每日 [startTime, endTime]（相对 0 点的秒数）。
func IsInDayTimeRange(nowTime, startTime, endTime int64) bool {
	if startTime < 0 || startTime > secondsPerDay || endTime < 0 || endTime > secondsPerDay {
		return false
	}
	zero := ZeroHourUnix(nowTime)
	sec := nowTime - zero
	return sec >= startTime && sec <= endTime
}

// InSameWeek 是否同一 ISO 周。
func InSameWeek(t1, t2 int64) bool {
	loc := Now().Location()
	y1, w1 := time.Unix(t1, 0).In(loc).ISOWeek()
	y2, w2 := time.Unix(t2, 0).In(loc).ISOWeek()
	return y1 == y2 && w1 == w2
}

// InSameMonth 是否同一自然月。
func InSameMonth(t1, t2 int64) bool {
	loc := Now().Location()
	d1 := time.Unix(t1, 0).In(loc)
	d2 := time.Unix(t2, 0).In(loc)
	return d1.Year() == d2.Year() && d1.Month() == d2.Month()
}

// InSameCalendarDay 是否同一自然日（本地时区）。
func InSameCalendarDay(t1, t2 int64) bool {
	loc := Now().Location()
	d1 := time.Unix(t1, 0).In(loc)
	d2 := time.Unix(t2, 0).In(loc)
	return d1.Year() == d2.Year() && d1.Month() == d2.Month() && d1.Day() == d2.Day()
}
