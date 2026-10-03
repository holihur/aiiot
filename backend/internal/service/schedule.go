package service

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// cronField matchers: standard 5-field cron (min hour dom month dow).
// Supported syntax: * | n | n-m | */step | a,b | a-b/step.

type cronField struct {
	values map[int]bool // explicit set when not wildcard
	any    bool
}

func parseCronField(text string, min, max int) (*cronField, error) {
	f := &cronField{values: map[int]bool{}}
	text = strings.TrimSpace(text)
	if text == "" || text == "*" {
		f.any = true
		return f, nil
	}
	for _, part := range strings.Split(text, ",") {
		part = strings.TrimSpace(part)
		step := 1
		if sp := strings.SplitN(part, "/", 2); len(sp) == 2 {
			var err error
			step, err = strconv.Atoi(sp[1])
			if err != nil || step < 1 {
				return nil, fmt.Errorf("bad step in %q", part)
			}
			part = sp[0]
		}
		var lo, hi int
		switch {
		case part == "*" || part == "":
			lo, hi = min, max // wildcard base for */step
		case strings.Contains(part, "-"):
			rng := strings.SplitN(part, "-", 2)
			lo, _ = strconv.Atoi(rng[0])
			hi, _ = strconv.Atoi(rng[1])
		default:
			lo, _ = strconv.Atoi(part)
			hi = lo
		}
		if lo < min || hi > max || lo > hi {
			return nil, fmt.Errorf("field out of range %d-%d: %q", min, max, part)
		}
		for v := lo; v <= hi; v += step {
			f.values[v] = true
		}
	}
	return f, nil
}

func (f *cronField) match(v int) bool { return f == nil || f.any || f.values[v] }

// dayNames maps 3-letter weekday names to cron numbers (0 = Sunday).
var dayNames = map[string]int{
	"SUN": 0, "MON": 1, "TUE": 2, "WED": 3, "THU": 4, "FRI": 5, "SAT": 6,
}

// parseCron parses a 5-field expression into matchers (weekday names allowed
// in the dow field).
func parseCron(expr string) ([]*cronField, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("cron must have 5 fields (min hour dom month dow): %q", expr)
	}
	bounds := [][2]int{{0, 59}, {0, 23}, {1, 31}, {1, 12}, {0, 6}}
	out := make([]*cronField, 5)
	for i := range fields {
		txt := fields[i]
		if i == 4 {
			txt = expandDayNames(txt)
		}
		f, err := parseCronField(txt, bounds[i][0], bounds[i][1])
		if err != nil {
			return nil, err
		}
		out[i] = f
	}
	return out, nil
}

// expandDayNames replaces weekday names in a dow field with numbers so ranges
// and lists like MON-FRI or MON,WED keep working.
func expandDayNames(txt string) string {
	parts := strings.Split(txt, ",")
	for i, p := range parts {
		parts[i] = expandRangeNames(p)
	}
	return strings.Join(parts, ",")
}

func expandRangeNames(p string) string {
	if strings.Contains(p, "-") {
		rng := strings.SplitN(p, "-", 2)
		a, okA := dayNames[strings.ToUpper(rng[0])]
		b, okB := dayNames[strings.ToUpper(rng[1])]
		if okA && okB {
			return strconv.Itoa(a) + "-" + strconv.Itoa(b)
		}
	}
	if step := strings.SplitN(p, "/", 2); len(step) == 2 {
		if v, ok := dayNames[strings.ToUpper(step[0])]; ok {
			return strconv.Itoa(v) + "/" + step[1]
		}
	}
	if v, ok := dayNames[strings.ToUpper(p)]; ok {
		return strconv.Itoa(v)
	}
	return p
}

// cronMatch reports whether a parsed expression fires at the given time. A
// literal day-of-month or day-of-week narrows the other (standard cron: if
// both are non-*, either matching is accepted).
func cronMatch(fields []*cronField, t time.Time) bool {
	dom, dow := t.Day(), int(t.Weekday())
	domM, dowM := fields[2].match(dom), fields[4].match(dow)
	// Day matching per standard cron: an explicit dom or dow restricts the
	// other one; when both are explicit, either matching is accepted.
	domAny, dowAny := fields[2].any, fields[4].any
	var dayM bool
	switch {
	case domAny && dowAny:
		dayM = true
	case domAny:
		dayM = dowM
	case dowAny:
		dayM = domM
	default:
		dayM = domM || dowM
	}
	return fields[0].match(t.Minute()) && fields[1].match(t.Hour()) &&
		dayM && fields[3].match(int(t.Month()))
}

// CronMatches parses and matches an expression (cached per rule by the caller
// where hot).
func CronMatches(expr string, t time.Time) bool {
	fields, err := parseCron(expr)
	if err != nil {
		return false
	}
	return cronMatch(fields, t)
}
