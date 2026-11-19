package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/razoreater/trnr/internal/garmin"
)

func main() {
	n := flag.Int("n", 10, "number of activities to list")
	activityType := flag.String("type", "", "only include this activity type, e.g. running or cycling")
	weeks := flag.Int("weeks", 4, "number of weeks in the weekly totals (0 to hide them)")
	flag.Parse()

	if *n < 0 || *weeks < 0 {
		fmt.Fprintln(os.Stderr, "-n and -weeks must be 0 or more")
		os.Exit(2)
	}

	apiKey := os.Getenv("GARMIN_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "GARMIN_API_KEY is not set (did you source .env?)")
		os.Exit(1)
	}

	c := garmin.New(apiKey)
	if err := c.Authenticate(false); err != nil {
		fmt.Fprintln(os.Stderr, "get token:", err)
		os.Exit(1)
	}

	// The weekly totals start on Monday of the oldest requested week.
	from := weekStart(time.Now()).AddDate(0, 0, -7*(*weeks-1))
	if *weeks == 0 {
		from = time.Now()
	}

	activities, err := c.FetchActivities(*n, *activityType, from)
	if errors.Is(err, garmin.ErrUnauthorized) {
		// The cached token was revoked or expired early: get a new one, try once more.
		if err := c.Authenticate(true); err != nil {
			fmt.Fprintln(os.Stderr, "get token:", err)
			os.Exit(1)
		}
		activities, err = c.FetchActivities(*n, *activityType, from)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "get activities:", err)
		os.Exit(1)
	}

	printTable(os.Stdout, activities[:min(*n, len(activities))])

	if *weeks > 0 {
		fmt.Println()
		printWeeks(os.Stdout, weeklyTotals(activities, from, *weeks))
	}
}
