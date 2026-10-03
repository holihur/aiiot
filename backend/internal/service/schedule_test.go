package service

import (
	"testing"
	"time"
)

func cronAt(expr string, tm string, tz string) bool {
	loc := time.UTC
	if tz != "" {
		loc = time.FixedZone("X", 8*3600)
	}
	t, err := time.ParseInLocation("2006-01-02 15:04:05", tm, loc)
	if err != nil {
		panic(err)
	}
	return CronMatches(expr, t)
}

func TestCronEveryMinute(t *testing.T) {
	if !cronAt("* * * * *", "2026-10-03 08:00:00", "") {
		t.Fatal("* * * * * must always match")
	}
	if !cronAt("* * * * *", "2026-10-03 23:59:59", "") {
		t.Fatal("* * * * * must match 23:59")
	}
}

func TestCronStepAndValue(t *testing.T) {
	if !cronAt("*/5 * * * *", "2026-10-03 08:05:00", "") {
		t.Fatal("*/5 must match :05")
	}
	if cronAt("*/5 * * * *", "2026-10-03 08:07:00", "") {
		t.Fatal("*/5 must not match :07")
	}
	if !cronAt("30 9 * * *", "2026-10-03 09:30:00", "") {
		t.Fatal("09:30 must match")
	}
	if cronAt("30 9 * * *", "2026-10-03 09:31:00", "") {
		t.Fatal("09:31 must not match")
	}
}

func TestCronRangeAndList(t *testing.T) {
	if !cronAt("0 9-17 * * MON-FRI", "2026-10-05 10:00:00", "") { // Monday
		t.Fatal("Mon 10:00 in range must match")
	}
	if cronAt("0 9-17 * * MON-FRI", "2026-10-10 10:00:00", "") { // Saturday
		t.Fatal("Sat must not match MON-FRI")
	}
	if !cronAt("0 8,20 * * *", "2026-10-03 20:00:00", "") {
		t.Fatal("20:00 in list must match")
	}
	if cronAt("0 8,20 * * *", "2026-10-03 15:00:00", "") {
		t.Fatal("15:00 must not match")
	}
}

func TestCronDaySemantics(t *testing.T) {
	// day-only: dom restricted, dow wildcard
	if !cronAt("0 12 1 * *", "2026-10-01 12:00:00", "") {
		t.Fatal("dom 1 must match on the 1st")
	}
	if cronAt("0 12 1 * *", "2026-10-02 12:00:00", "") {
		t.Fatal("must not match on the 2nd")
	}
	// both restricted: either wins (Sun=0)
	if !cronAt("0 12 15 10 0", "2026-10-18 12:00:00", "utc") { // 2026-10-18 is a Sunday
		t.Fatal("either dom or dow should match (oct 18 = sunday)")
	}
}

func TestCronBadExpr(t *testing.T) {
	if CronMatches("61 * * * *", time.Now()) {
		t.Fatal("minute 61 invalid")
	}
	if CronMatches("not cron", time.Now()) {
		t.Fatal("invalid arity")
	}
}
