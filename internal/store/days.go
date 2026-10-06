package store

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const layout = "2006-01-02"

// Today is the local calendar date. All dates in task files are local dates,
// never timestamps, so a task done at 23:50 stays on that day.
func Today() string { return time.Now().Format(layout) }

func AddDays(day string, n int) string {
	t, err := time.ParseInLocation(layout, day, time.Local)
	if err != nil {
		return day
	}
	return t.AddDate(0, 0, n).Format(layout)
}

// Label renders 2026-10-06 as "tue 06".
func Label(day string) string {
	t, err := time.ParseInLocation(layout, day, time.Local)
	if err != nil {
		return day
	}
	return strings.ToLower(t.Format("Mon 02"))
}

// LongLabel renders 2026-10-06 as "tue 06 oct".
func LongLabel(day string) string {
	t, err := time.ParseInLocation(layout, day, time.Local)
	if err != nil {
		return day
	}
	return strings.ToLower(t.Format("Mon 02 Jan"))
}

// ParseDay accepts t/today, m/tomorrow, y/yesterday, +N, -N, a weekday
// (next one after today), YYYY-MM-DD, or MM-DD (this year). Empty, "-" and
// "none" return "" which means unset.
func ParseDay(in, today string) (string, error) {
	in = strings.ToLower(strings.TrimSpace(in))
	switch in {
	case "", "-", "none":
		return "", nil
	case "t", "today":
		return today, nil
	case "m", "tom", "tomorrow":
		return AddDays(today, 1), nil
	case "y", "yesterday":
		return AddDays(today, -1), nil
	}
	if in[0] == '+' || in[0] == '-' {
		if n, err := strconv.Atoi(in); err == nil {
			return AddDays(today, n), nil
		}
	}
	base, _ := time.ParseInLocation(layout, today, time.Local)
	for i := 1; i <= 7; i++ {
		d := base.AddDate(0, 0, i)
		name := strings.ToLower(d.Weekday().String())
		if len(in) >= 2 && strings.HasPrefix(name, in) {
			return d.Format(layout), nil
		}
	}
	if t, err := time.ParseInLocation(layout, in, time.Local); err == nil {
		return t.Format(layout), nil
	}
	if t, err := time.ParseInLocation("2006-01-02", fmt.Sprintf("%d-%s", base.Year(), in), time.Local); err == nil {
		return t.Format(layout), nil
	}
	return "", fmt.Errorf("can't read day %q (try t, m, +3, fri, 2026-10-12)", in)
}

type Placement struct {
	Day         string // calendar column; "" means backlog
	CarriedFrom string // set when an unfinished plan slipped past its day
}

// Place decides which calendar column a task belongs in. A finished task sits
// on the day it was finished, not the day it was planned. An unfinished task
// planned for a past day moves to today and remembers where it came from.
func Place(t Task, today string) Placement {
	switch {
	case t.Done():
		return Placement{Day: t.DoneAt}
	case t.Planned == "":
		return Placement{}
	case t.Planned < today:
		return Placement{Day: today, CarriedFrom: t.Planned}
	default:
		return Placement{Day: t.Planned}
	}
}

type DeadlineState int

const (
	DeadlineNone DeadlineState = iota
	DeadlineLater
	DeadlineSoon // within 2 days
	DeadlinePassed
)

func Deadline(t Task, today string) DeadlineState {
	switch {
	case t.Deadline == "" || t.Done():
		return DeadlineNone
	case t.Deadline < today:
		return DeadlinePassed
	case t.Deadline <= AddDays(today, 2):
		return DeadlineSoon
	default:
		return DeadlineLater
	}
}
