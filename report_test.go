package main

import (
	"bytes"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/razoreater/trnr/internal/garmin"
)

// activity builds a test activity; only the fields the report uses.
func activity(typeKey, start string, metres, seconds, speed float64) garmin.Activity {
	var a garmin.Activity
	a.Type.TypeKey = typeKey
	a.StartTimeLocal = start
	a.Distance = metres
	a.Duration = seconds
	a.AverageSpeed = speed
	return a
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		sec  float64
		want string
	}{
		{0, "0:00"},
		{59, "0:00"}, // seconds are dropped, not rounded
		{60, "0:01"},
		{3599, "0:59"},
		{3600, "1:00"},
		{5025.7, "1:23"},
		{36000, "10:00"},
	}
	for _, tt := range tests {
		if got := formatDuration(tt.sec); got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.sec, got, tt.want)
		}
	}
}

func TestFormatKm(t *testing.T) {
	tests := []struct {
		metres float64
		want   string
	}{
		{0, "0.0"},
		{999, "1.0"},
		{10000, "10.0"},
		{42195, "42.2"},
	}
	for _, tt := range tests {
		if got := formatKm(tt.metres); got != tt.want {
			t.Errorf("formatKm(%v) = %q, want %q", tt.metres, got, tt.want)
		}
	}
}

func TestFormatPace(t *testing.T) {
	tests := []struct {
		name    string
		typeKey string
		speed   float64 // m/s
		want    string
	}{
		{"running 5:00", "running", 1000.0 / 300, "5:00 /km"},
		{"running 6:40", "running", 2.5, "6:40 /km"},
		// 1000/(1000/195) is 194.99999999999997 in floating point.
		{"running 3:15 rounds, not truncates", "running", 1000.0 / 195, "3:15 /km"},
		{"running rounds to nearest second", "running", 1000 / 299.6, "5:00 /km"},
		{"walking uses pace", "walking", 1000.0 / 600, "10:00 /km"},
		{"running without speed", "running", 0, "-"},
		{"cycling", "cycling", 25 / 3.6, "25.0 km/h"},
		{"cycling without speed", "cycling", 0, "-"},
		{"other type", "strength_training", 1.5, "-"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := activity(tt.typeKey, "", 0, 0, tt.speed)
			if got := formatPace(a); got != tt.want {
				t.Errorf("formatPace(%s, %v m/s) = %q, want %q", tt.typeKey, tt.speed, got, tt.want)
			}
		})
	}
}

func TestWeekKey(t *testing.T) {
	tests := []struct {
		date string
		want string
	}{
		{"2026-11-16", "2026-W47"},
		{"2026-01-01", "2026-W01"},
		// ISO weeks belong to the year that has their Thursday, so these
		// dates sit in a week of a different calendar year.
		{"2027-01-03", "2026-W53"},
		{"2027-01-04", "2027-W01"},
		{"2025-12-29", "2026-W01"},
	}
	for _, tt := range tests {
		date, err := time.Parse("2006-01-02", tt.date)
		if err != nil {
			t.Fatal(err)
		}
		if got := weekKey(date); got != tt.want {
			t.Errorf("weekKey(%s) = %q, want %q", tt.date, got, tt.want)
		}
	}
}

func TestWeekStart(t *testing.T) {
	monday := time.Date(2026, 11, 16, 0, 0, 0, 0, time.Local)
	tests := []time.Time{
		monday,
		time.Date(2026, 11, 16, 23, 59, 0, 0, time.Local), // late Monday
		time.Date(2026, 11, 19, 12, 0, 0, 0, time.Local),  // Thursday
		time.Date(2026, 11, 22, 18, 30, 0, 0, time.Local), // Sunday
	}
	for _, in := range tests {
		if got := weekStart(in); !got.Equal(monday) {
			t.Errorf("weekStart(%s) = %s, want %s", in.Format(time.DateTime), got, monday)
		}
	}
}

func TestWeeklyTotals(t *testing.T) {
	from := time.Date(2026, 11, 2, 0, 0, 0, 0, time.Local) // Monday of W45
	activities := []garmin.Activity{
		activity("running", "2026-11-17 07:00:00", 10000, 3000, 0), // W47
		activity("cycling", "2026-11-16 18:00:00", 30000, 3600, 0), // W47
		activity("running", "2026-11-03 07:00:00", 5000, 1500, 0),  // W45
		activity("running", "2026-10-30 07:00:00", 8000, 2400, 0),  // W44: too old
		activity("running", "not a date", 1000, 100, 0),            // skipped
	}

	totals := weeklyTotals(activities, from, 3)

	if len(totals) != 3 {
		t.Fatalf("got %d weeks, want 3: %v", len(totals), totals)
	}
	want := map[string]weekTotal{
		"2026-W45": {count: 1, distance: 5000, duration: 1500},
		"2026-W46": {}, // a week without activities is still there
		"2026-W47": {count: 2, distance: 40000, duration: 6600},
	}
	for key, w := range want {
		got, ok := totals[key]
		if !ok {
			t.Errorf("missing week %s", key)
			continue
		}
		if got.count != w.count || got.distance != w.distance || got.duration != w.duration {
			t.Errorf("%s = %+v, want count %d, distance %v, duration %v",
				key, *got, w.count, w.distance, w.duration)
		}
	}
}

func TestActivityRows(t *testing.T) {
	run := activity("running", "2026-11-17 07:00:00", 10000, 3000, 1000.0/300)
	run.Name = "Morning Run"
	run.AverageHR = 148
	run.AerobicTE = 3.2
	yoga := activity("yoga", "2026-11-18 19:30:00", 0, 2700, 0)
	yoga.Name = "Evening Yoga"

	got := activityRows([]garmin.Activity{run, yoga})

	want := [][]string{
		{"Tue 17 Nov", "running", "Morning Run", "10.0", "0:50", "5:00 /km", "148", "3.2"},
		{"Wed 18 Nov", "yoga", "Evening Yoga", "0.0", "0:45", "-", "0", "0.0"},
	}
	if !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("activityRows =\n%q\nwant\n%q", got, want)
	}
	for _, row := range got {
		if len(row) != len(activityHeaders) {
			t.Errorf("row has %d cells, headers has %d", len(row), len(activityHeaders))
		}
	}
}

func TestWeekRows(t *testing.T) {
	totals := map[string]*weekTotal{
		"2026-W47": {start: time.Date(2026, 11, 16, 0, 0, 0, 0, time.Local), count: 2, distance: 40000, duration: 6600},
		"2026-W45": {start: time.Date(2026, 11, 2, 0, 0, 0, 0, time.Local), count: 1, distance: 5000, duration: 1500},
		"2026-W46": {start: time.Date(2026, 11, 9, 0, 0, 0, 0, time.Local)},
	}

	got := weekRows(totals)

	// Map order is random in Go, so the rows must be sorted by week.
	want := [][]string{
		{"2026-W45", "02 Nov", "1", "5.0", "0:25"},
		{"2026-W46", "09 Nov", "0", "0.0", "0:00"},
		{"2026-W47", "16 Nov", "2", "40.0", "1:50"},
	}
	if !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("weekRows =\n%q\nwant\n%q", got, want)
	}
}

// The rendering tests compare the whole output. A bytes.Buffer is not a
// terminal, so lipgloss must leave out all colors (ANSI escape codes).

func TestPrintTable(t *testing.T) {
	run := activity("running", "2026-11-17 07:00:00", 10000, 3000, 1000.0/300)
	run.Name = "Morning Run"
	run.AverageHR = 148
	run.AerobicTE = 3.2
	ride := activity("cycling", "2026-11-18 17:00:00", 30000, 3600, 30/3.6)
	ride.Name = "Commute"
	ride.AverageHR = 121
	ride.AerobicTE = 1.4

	var out bytes.Buffer
	printTable(&out, []garmin.Activity{run, ride})

	want := `╭────────────┬─────────┬─────────────┬──────┬──────┬───────────┬─────┬─────╮
│ DATE       │ TYPE    │ NAME        │   KM │ TIME │      PACE │  HR │  TE │
├────────────┼─────────┼─────────────┼──────┼──────┼───────────┼─────┼─────┤
│ Tue 17 Nov │ running │ Morning Run │ 10.0 │ 0:50 │  5:00 /km │ 148 │ 3.2 │
│ Wed 18 Nov │ cycling │ Commute     │ 30.0 │ 1:00 │ 30.0 km/h │ 121 │ 1.4 │
╰────────────┴─────────┴─────────────┴──────┴──────┴───────────┴─────┴─────╯
`
	if strings.Contains(out.String(), "\x1b") {
		t.Errorf("output has ANSI escape codes, want plain text:\n%q", out.String())
	}
	if out.String() != want {
		t.Errorf("printTable output:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestPrintWeeks(t *testing.T) {
	totals := map[string]*weekTotal{
		"2026-W46": {start: time.Date(2026, 11, 9, 0, 0, 0, 0, time.Local), count: 18, distance: 68000, duration: 38160},
		"2026-W47": {start: time.Date(2026, 11, 16, 0, 0, 0, 0, time.Local), count: 9, distance: 41100, duration: 22260},
	}

	var out bytes.Buffer
	printWeeks(&out, totals)

	want := `╭──────────────────────────────────────╮
│ Weekly totals                        │
│ WEEK      FROM    COUNT    KM   TIME │
│──────────────────────────────────────│
│ 2026-W46  09 Nov     18  68.0  10:36 │
│ 2026-W47  16 Nov      9  41.1   6:11 │
╰──────────────────────────────────────╯
`
	if strings.Contains(out.String(), "\x1b") {
		t.Errorf("output has ANSI escape codes, want plain text:\n%q", out.String())
	}
	if out.String() != want {
		t.Errorf("printWeeks output:\n%s\nwant:\n%s", out.String(), want)
	}
}
