package controllers

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Minimal cron schedule support for the CronJob controller: the standard
// 5-field expression (minute hour day-of-month month day-of-week) plus the
// "@every <duration>" form. Fields accept "*", lists (","), ranges ("a-b"),
// steps ("* /n", "a-b/n", "a/n") and 3-letter month/day names.

// Schedule describes a parsed CronJob schedule.
type Schedule struct {
	every  time.Duration // > 0 for "@every" schedules
	minute cronField
	hour   cronField
	dom    cronField
	month  cronField
	dow    cronField
	domSet bool // day-of-month is restricted
	dowSet bool // day-of-week is restricted
}

type cronField struct {
	bits map[int]bool
}

func (f cronField) match(v int) bool {
	if f.bits == nil {
		return true
	}
	return f.bits[v]
}

var monthNames = map[string]int{
	"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6,
	"jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12,
}

var dowNames = map[string]int{
	"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6,
}

// ParseSchedule parses a CronJob schedule expression.
func ParseSchedule(expr string) (*Schedule, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, fmt.Errorf("cron: empty schedule")
	}
	if d, ok := strings.CutPrefix(expr, "@every "); ok {
		dur, err := time.ParseDuration(strings.TrimSpace(d))
		if err != nil {
			return nil, fmt.Errorf("cron: @every %q: %w", d, err)
		}
		if dur <= 0 {
			return nil, fmt.Errorf("cron: @every must be positive")
		}
		return &Schedule{every: dur}, nil
	}

	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("cron: %q: expected 5 fields (minute hour day-of-month month day-of-week), got %d", expr, len(fields))
	}

	var s Schedule
	var err error
	if s.minute, err = parseCronField(fields[0], 0, 59, nil); err != nil {
		return nil, fmt.Errorf("cron: minute: %w", err)
	}
	if s.hour, err = parseCronField(fields[1], 0, 23, nil); err != nil {
		return nil, fmt.Errorf("cron: hour: %w", err)
	}
	if s.dom, err = parseCronField(fields[2], 1, 31, nil); err != nil {
		return nil, fmt.Errorf("cron: day-of-month: %w", err)
	}
	if s.month, err = parseCronField(fields[3], 1, 12, monthNames); err != nil {
		return nil, fmt.Errorf("cron: month: %w", err)
	}
	if s.dow, err = parseCronField(fields[4], 0, 7, dowNames); err != nil {
		return nil, fmt.Errorf("cron: day-of-week: %w", err)
	}
	// 7 is an alias for Sunday.
	if s.dow.bits != nil && s.dow.match(7) {
		s.dow.bits[0] = true
	}
	s.domSet = fields[2] != "*"
	s.dowSet = fields[4] != "*"
	return &s, nil
}

// parseCronField parses one cron field into a bit set.
func parseCronField(spec string, min, max int, names map[string]int) (cronField, error) {
	f := cronField{}
	if spec == "*" {
		return f, nil // nil bits match everything
	}
	f.bits = make(map[int]bool)
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		step := 1
		if base, st, ok := strings.Cut(part, "/"); ok {
			n, err := strconv.Atoi(st)
			if err != nil || n <= 0 {
				return f, fmt.Errorf("bad step %q", st)
			}
			step = n
			part = base
		}
		lo, hi := min, max
		switch {
		case part == "*" || part == "":
			// full range with the step
		case strings.Contains(part, "-"):
			a, b, _ := strings.Cut(part, "-")
			var err error
			if lo, err = parseCronValue(a, min, max, names); err != nil {
				return f, err
			}
			if hi, err = parseCronValue(b, min, max, names); err != nil {
				return f, err
			}
		default:
			v, err := parseCronValue(part, min, max, names)
			if err != nil {
				return f, err
			}
			lo, hi = v, v
			if step > 1 {
				hi = max // "a/n" runs from a to the end of the range
			}
		}
		for v := lo; v <= hi; v += step {
			f.bits[v] = true
		}
	}
	if len(f.bits) == 0 {
		return f, fmt.Errorf("field %q matches nothing", spec)
	}
	return f, nil
}

func parseCronValue(s string, min, max int, names map[string]int) (int, error) {
	s = strings.TrimSpace(s)
	if names != nil {
		if v, ok := names[strings.ToLower(s)]; ok {
			return v, nil
		}
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("bad value %q", s)
	}
	// Day-of-week accepts 7 as Sunday; nothing else may exceed its range.
	if v < min || v > max {
		return 0, fmt.Errorf("value %d out of range [%d,%d]", v, min, max)
	}
	return v, nil
}

// Matches reports whether t (truncated to the minute) satisfies the schedule.
func (s *Schedule) Matches(t time.Time) bool {
	if s == nil {
		return false
	}
	if s.every > 0 {
		return false // @every schedules are driven by Next only
	}
	t = t.Truncate(time.Minute)
	if !s.minute.match(t.Minute()) || !s.hour.match(t.Hour()) || !s.month.match(int(t.Month())) {
		return false
	}
	domOK := s.dom.match(t.Day())
	dowOK := s.dow.match(int(t.Weekday()))
	// Vixie cron semantics: when both day fields are restricted the entry
	// matches when either of them matches.
	if s.domSet && s.dowSet {
		return domOK || dowOK
	}
	return domOK && dowOK
}

// Next returns the first activation strictly after t, or the zero time when no
// activation exists. "@every" schedules return t+every; cron expressions are
// matched minute by minute (bounded to five years).
func (s *Schedule) Next(t time.Time) time.Time {
	if s == nil {
		return time.Time{}
	}
	if s.every > 0 {
		return t.Add(s.every)
	}
	cur := t.Truncate(time.Minute).Add(time.Minute)
	limit := t.AddDate(5, 0, 0)
	for cur.Before(limit) {
		if s.Matches(cur) {
			return cur
		}
		cur = cur.Add(time.Minute)
	}
	return time.Time{}
}
