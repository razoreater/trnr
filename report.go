package main

import (
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/razoreater/trnr/internal/garmin"
)

func formatDuration(sec float64) string {
	total := int(sec)
	hours := total / 3600
	minutes := (total % 3600) / 60
	return fmt.Sprintf("%d:%02d", hours, minutes)
}

func formatKm(meter float64) string {
	return fmt.Sprintf("%.1f", meter/1000)
}

func formatDate(a garmin.Activity) string {
	t, err := a.Start()
	if err != nil {
		return a.StartTimeLocal
	}
	return t.Format("Mon 02 Jan")
}

func formatPace(a garmin.Activity) string {
	switch a.Type.TypeKey {
	case "running", "walking":
		if a.AverageSpeed == 0 {
			return "-"
		}
		secPerKm := int(math.Round(1000 / a.AverageSpeed))
		minutes := secPerKm / 60
		seconds := secPerKm % 60
		return fmt.Sprintf("%d:%02d /km", minutes, seconds)

	case "cycling":
		if a.AverageSpeed == 0 {
			return "-"
		}
		return fmt.Sprintf("%.1f km/h", a.AverageSpeed*3.6)

	default:
		return "-"
	}
}

var activityHeaders = []string{"DATE", "TYPE", "NAME", "KM", "TIME", "PACE", "HR", "TE"}

// activityRows formats each activity as one table row of text cells.
func activityRows(activities []garmin.Activity) [][]string {
	rows := make([][]string, 0, len(activities))
	for _, a := range activities {
		rows = append(rows, []string{
			formatDate(a),
			a.Type.TypeKey,
			a.Name,
			formatKm(a.Distance),
			formatDuration(a.Duration),
			formatPace(a),
			fmt.Sprintf("%.0f", a.AverageHR),
			fmt.Sprintf("%.1f", a.AerobicTE),
		})
	}
	return rows
}

type weekTotal struct {
	start    time.Time // Monday 00:00 of the week
	count    int
	distance float64 // metres
	duration float64 // seconds
}

// weekKey returns the ISO week of t as "2026-W46". The zero-padded week
// number makes these keys sort in date order.
func weekKey(t time.Time) string {
	year, week := t.ISOWeek()
	return fmt.Sprintf("%d-W%02d", year, week)
}

// weekStart returns Monday 00:00 of the week that contains t.
func weekStart(t time.Time) time.Time {
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	daysSinceMonday := (int(day.Weekday()) + 6) % 7 // Sunday is 0 in Go
	return day.AddDate(0, 0, -daysSinceMonday)
}

// weeklyTotals sums activities per ISO week for the given number of weeks
// starting at from (a Monday). Weeks without activities are included with zeros.
func weeklyTotals(activities []garmin.Activity, from time.Time, weeks int) map[string]*weekTotal {
	totals := make(map[string]*weekTotal)
	for i := range weeks {
		start := from.AddDate(0, 0, 7*i)
		totals[weekKey(start)] = &weekTotal{start: start}
	}

	for _, a := range activities {
		t, err := a.Start()
		if err != nil {
			continue
		}
		total, ok := totals[weekKey(t)]
		if !ok {
			continue // outside the requested weeks
		}
		total.count++
		total.distance += a.Distance
		total.duration += a.Duration
	}
	return totals
}

var weekHeaders = []string{"WEEK", "FROM", "COUNT", "KM", "TIME"}

// weekRows formats the weekly totals as table rows, oldest week first.
func weekRows(totals map[string]*weekTotal) [][]string {
	keys := make([]string, 0, len(totals))
	for key := range totals {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	rows := make([][]string, 0, len(keys))
	for _, key := range keys {
		total := totals[key]
		rows = append(rows, []string{
			key,
			total.start.Format("02 Jan"),
			fmt.Sprint(total.count),
			formatKm(total.distance),
			formatDuration(total.duration),
		})
	}
	return rows
}
