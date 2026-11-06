package main

import (
	"fmt"
	"net/http"
	"os"
)

func main() {
	apiKey := os.Getenv("GARMIN_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "GARMIN_API_KEY is not set (did you source .env?)")
		os.Exit(1)
	}

	token, err := getToken(apiKey)
	if err != nil {
		fmt.Fprintln(os.Stderr, "get token:", err)
		os.Exit(1)
	}

	c := &client{http: http.DefaultClient, token: token, baseURL: baseURL}
	activities, err := c.getActivities(5)
	if err != nil {
		fmt.Fprintln(os.Stderr, "get activities:", err)
		os.Exit(1)
	}

	printTable(activities)
}
