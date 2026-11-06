package main

import (
	"fmt"
	"os"
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

func formatDate(s string) string {
	t, err := time.Parse("2006-01-02 15:04:05", s)
	if err != nil {
		return s
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
			formatDate(a.StartTimeLocal),
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
