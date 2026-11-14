package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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

// errUnauthorized means the API rejected the token (HTTP 401).
var errUnauthorized = errors.New("unauthorized")

// cachedToken is what we store in ~/.cache/trnr/token.json.
type cachedToken struct {
	Token   string    `json:"token"`
	Expires time.Time `json:"expires"`
}

func tokenCachePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("find cache dir: %w", err)
	}
	return filepath.Join(dir, "trnr", "token.json"), nil
}

// loadCachedToken returns the cached token if there is one that is still
// valid for at least a few minutes. Any problem just means "no token".
func loadCachedToken() (string, bool) {
	path, err := tokenCachePath()
	if err != nil {
		return "", false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	var cached cachedToken
	if err := json.Unmarshal(data, &cached); err != nil {
		return "", false
	}
	if cached.Token == "" || time.Now().Add(5*time.Minute).After(cached.Expires) {
		return "", false
	}
	return cached.Token, true
}

func saveToken(token string, expires time.Time) error {
	path, err := tokenCachePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}
	data, err := json.Marshal(cachedToken{Token: token, Expires: expires})
	if err != nil {
		return fmt.Errorf("encode token: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write token cache: %w", err)
	}
	// WriteFile only sets the mode when it creates the file, so enforce it.
	return os.Chmod(path, 0o600)
}

// tokenExpiry reads the "exp" claim from the JWT's payload (the middle part,
// base64url-encoded JSON). If that fails it assumes the documented 24h.
func tokenExpiry(token string) time.Time {
	fallback := time.Now().Add(24 * time.Hour)

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return fallback
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return fallback
	}
	var claims struct {
		Exp int64 `json:"exp"` // seconds since 1970
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp == 0 {
		return fallback
	}
	return time.Unix(claims.Exp, 0)
}

// accessToken returns a cached token when possible. With refresh set, or
// without a valid cached token, it fetches a new one and caches it.
func accessToken(apiKey string, refresh bool) (string, error) {
	if !refresh {
		if token, ok := loadCachedToken(); ok {
			return token, nil
		}
	}
	token, err := getToken(apiKey)
	if err != nil {
		return "", err
	}
	if err := saveToken(token, tokenExpiry(token)); err != nil {
		// Not fatal: we have a token, it just won't be reused next time.
		fmt.Fprintln(os.Stderr, "warning: cache token:", err)
	}
	return token, nil
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

// getActivities fetches one page of activities, newest first. An empty
// activityType means all types.
func (c *client) getActivities(start, limit int, activityType string) ([]Activity, error) {
	query := url.Values{}
	query.Set("start", strconv.Itoa(start))
	query.Set("limit", strconv.Itoa(limit))
	if activityType != "" {
		query.Set("activityType", activityType)
	}
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

	if response.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("activities request: %w", errUnauthorized)
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

// fetchActivities pages through the activity list until it has at least n
// activities and has gone back past since, or there are no more activities.
func (c *client) fetchActivities(n int, activityType string, since time.Time) ([]Activity, error) {
	const pageSize = 50
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
		oldest, err := parseStart(all[len(all)-1])
		if err != nil {
			return nil, err
		}
		if len(all) >= n && oldest.Before(since) {
			return all, nil
		}
	}
}
