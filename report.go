package main

import (
	"fmt"
	"os"
	"slices"
	"text/tabwriter"
	"time"
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

// parseStart parses an activity's local start time ("2006-01-02 15:04:05").
func parseStart(a Activity) (time.Time, error) {
	return time.ParseInLocation("2006-01-02 15:04:05", a.StartTimeLocal, time.Local)
}

func formatDate(a Activity) string {
	t, err := parseStart(a)
	if err != nil {
		return a.StartTimeLocal
	}
	return t.Format("Mon 02 Jan")
}

func formatPace(a Activity) string {
	switch a.Type.TypeKey {
	case "running", "walking":
		if a.AverageSpeed == 0 {
			return "-"
		}
		secPerKm := int(1000 / a.AverageSpeed)
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

func printTable(activities []Activity) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "DATE\tTYPE\tNAME\tKM\tTIME\tPACE\tHR\tTE")

	for _, a := range activities {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%.0f\t%.1f\n",
			formatDate(a),
			a.Type.TypeKey,
			a.Name,
			formatKm(a.Distance),
			formatDuration(a.Duration),
			formatPace(a),
			a.AverageHR,
			a.AerobicTE,
		)
	}

	w.Flush()
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
func weeklyTotals(activities []Activity, from time.Time, weeks int) map[string]*weekTotal {
	totals := make(map[string]*weekTotal)
	for i := range weeks {
		start := from.AddDate(0, 0, 7*i)
		totals[weekKey(start)] = &weekTotal{start: start}
	}

	for _, a := range activities {
		t, err := parseStart(a)
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

func printWeeks(totals map[string]*weekTotal) {
	keys := make([]string, 0, len(totals))
	for key := range totals {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "WEEK\tFROM\tCOUNT\tKM\tTIME")
	for _, key := range keys {
		total := totals[key]
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\n",
			key,
			total.start.Format("02 Jan"),
			total.count,
			formatKm(total.distance),
			formatDuration(total.duration),
		)
	}
	w.Flush()
}
