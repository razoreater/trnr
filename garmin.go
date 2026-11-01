package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	baseURL  = "https://personal.connectapi.garmin.com"
	tokenURL = "https://personal.connectapi.garmin.com/personalapitoken-service/personal-api/personal-api-key/token"
)

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

type Activity struct {
	ActivityID     int64  `json:"activityId"`
	Name           string `json:"activityName"`
	StartTimeLocal string `json:"startTimeLocal"`
	Type           struct {
		TypeKey string `json:"typeKey"`
	} `json:"activityType"`
	Distance      float64 `json:"distance"`     // metres
	Duration      float64 `json:"duration"`     // seconds
	AverageSpeed  float64 `json:"averageSpeed"` // m/s
	AverageHR     float64 `json:"averageHR"`
	Calories      float64 `json:"calories"`
	ElevationGain float64 `json:"elevationGain"`
	AerobicTE     float64 `json:"aerobicTrainingEffect"`
	TrainingLoad  float64 `json:"activityTrainingLoad"`
}

type client struct {
	http    *http.Client
	token   string
	baseURL string
}

func (c *client) getActivities(limit int) ([]Activity, error) {
	query := url.Values{}
	query.Set("start", "0")
	query.Set("limit", strconv.Itoa(limit))
	activitiesURL := c.baseURL + "/activitylist-service/activities/search/activities?" + query.Encode()
	request, err := http.NewRequest(http.MethodGet, activitiesURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build activities request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.token)

	response, err := c.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("send activities request: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("activities request failed: status %d", response.StatusCode)
	}

	var activities []Activity
	if err := json.NewDecoder(response.Body).Decode(&activities); err != nil {
		return nil, fmt.Errorf("decode activities: %w", err)
	}
	return activities, nil
}
