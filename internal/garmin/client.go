// Package garmin is a small client for the Garmin Connect Personal API.
package garmin

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// DefaultBaseURL is the host for both the token exchange and the data calls.
const DefaultBaseURL = "https://personal.connectapi.garmin.com"

// ErrUnauthorized means the API rejected the token (HTTP 401).
var ErrUnauthorized = errors.New("unauthorized")

// Client talks to the API. Call Authenticate before the data methods.
type Client struct {
	HTTP      *http.Client
	BaseURL   string
	APIKey    string
	CachePath string // where the token is cached; "" disables the cache

	token string
}

// New returns a client for the real API that caches its token in the
// default cache path. If there is no cache dir, it works without a cache.
func New(apiKey string) *Client {
	cachePath, _ := DefaultCachePath()
	return &Client{
		HTTP:      http.DefaultClient,
		BaseURL:   DefaultBaseURL,
		APIKey:    apiKey,
		CachePath: cachePath,
	}
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

// Start parses the activity's local start time ("2006-01-02 15:04:05").
func (a Activity) Start() (time.Time, error) {
	return time.ParseInLocation("2006-01-02 15:04:05", a.StartTimeLocal, time.Local)
}

// getActivities fetches one page of activities, newest first. An empty
// activityType means all types.
func (c *Client) getActivities(start, limit int, activityType string) ([]Activity, error) {
	query := url.Values{}
	query.Set("start", strconv.Itoa(start))
	query.Set("limit", strconv.Itoa(limit))
	if activityType != "" {
		query.Set("activityType", activityType)
	}
	activitiesURL := c.BaseURL + "/activitylist-service/activities/search/activities?" + query.Encode()
	request, err := http.NewRequest(http.MethodGet, activitiesURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build activities request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.token)

	response, err := c.HTTP.Do(request)
	if err != nil {
		return nil, fmt.Errorf("send activities request: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("activities request: %w", ErrUnauthorized)
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("activities request failed: status %d", response.StatusCode)
	}

	var activities []Activity
	if err := json.NewDecoder(response.Body).Decode(&activities); err != nil {
		return nil, fmt.Errorf("decode activities: %w", err)
	}
	return activities, nil
}

// pageSize is how many activities FetchActivities asks for per request.
const pageSize = 50

// FetchActivities pages through the activity list until it has at least n
// activities and has gone back past since, or there are no more activities.
func (c *Client) FetchActivities(n int, activityType string, since time.Time) ([]Activity, error) {
	var all []Activity
	for {
		page, err := c.getActivities(len(all), pageSize, activityType)
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		if len(page) < pageSize {
			return all, nil
		}
		oldest, err := all[len(all)-1].Start()
		if err != nil {
			return nil, err
		}
		if len(all) >= n && oldest.Before(since) {
			return all, nil
		}
	}
}
