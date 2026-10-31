package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

const tokenURL = "https://personal.connectapi.garmin.com/personalapitoken-service/personal-api/personal-api-key/token"

func getToken(apiKey string) (string, error) {
	request, err := http.NewRequest(http.MethodPost, tokenURL, nil)
	if err != nil {
		return "", fmt.Errorf("build token request: %w", err)
	}

	request.Header.Set("Authorization", "ApiKey "+apiKey)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("send token request: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token request failed: status %d", response.StatusCode)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("read token response: %w", err)
	}
	return strings.TrimSpace(string(body)), nil
}
