package main

import (
	"fmt"
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
	fmt.Println("token OK, length", len(token))
}
