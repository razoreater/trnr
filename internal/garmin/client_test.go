package garmin

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

// newTestClient starts a fake API server with the given handler and returns
// a client pointed at it, with the token cache in a temp dir.
func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &Client{
		HTTP:      server.Client(),
		BaseURL:   server.URL,
		APIKey:    "test-key",
		CachePath: filepath.Join(t.TempDir(), "token.json"),
	}
}

// tokenHandler fakes the token endpoint and counts how often it is called.
func tokenHandler(t *testing.T, token string, calls *atomic.Int32) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != tokenPath {
			t.Errorf("got %s %s, want POST %s", r.Method, r.URL.Path, tokenPath)
		}
		if got := r.Header.Get("Authorization"); got != "ApiKey test-key" {
			t.Errorf("Authorization = %q, want %q", got, "ApiKey test-key")
		}
		fmt.Fprintln(w, token) // with a trailing newline, like the real API
	}
}

func TestAuthenticateRequestsAndCachesToken(t *testing.T) {
	var calls atomic.Int32
	c := newTestClient(t, tokenHandler(t, "tok-1", &calls))

	if err := c.Authenticate(false); err != nil {
		t.Fatal(err)
	}
	if c.token != "tok-1" {
		t.Errorf("token = %q, want %q (whitespace trimmed)", c.token, "tok-1")
	}

	info, err := os.Stat(c.CachePath)
	if err != nil {
		t.Fatalf("token not cached: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("cache file mode = %o, want 600", mode)
	}

	// A second client with the same cache must not ask for a new token.
	again := &Client{HTTP: c.HTTP, BaseURL: c.BaseURL, APIKey: c.APIKey, CachePath: c.CachePath}
	if err := again.Authenticate(false); err != nil {
		t.Fatal(err)
	}
	if again.token != "tok-1" {
		t.Errorf("cached token = %q, want %q", again.token, "tok-1")
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("token endpoint called %d times, want 1", n)
	}
}

func TestAuthenticateRefreshSkipsCache(t *testing.T) {
	var calls atomic.Int32
	c := newTestClient(t, tokenHandler(t, "new", &calls))
	if err := saveToken(c.CachePath, "old", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	if err := c.Authenticate(true); err != nil {
		t.Fatal(err)
	}
	if c.token != "new" || calls.Load() != 1 {
		t.Errorf("token = %q after %d calls, want a fresh token", c.token, calls.Load())
	}
}

func TestAuthenticateIgnoresExpiringCache(t *testing.T) {
	tests := []struct {
		name    string
		expires time.Duration // from now
	}{
		{"expired", -time.Hour},
		{"expires within 5 minutes", 2 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			c := newTestClient(t, tokenHandler(t, "new", &calls))
			if err := saveToken(c.CachePath, "old", time.Now().Add(tt.expires)); err != nil {
				t.Fatal(err)
			}

			if err := c.Authenticate(false); err != nil {
				t.Fatal(err)
			}
			if c.token != "new" {
				t.Errorf("token = %q, want a fresh one", c.token)
			}
		})
	}
}

func TestAuthenticateWithoutCache(t *testing.T) {
	var calls atomic.Int32
	c := newTestClient(t, tokenHandler(t, "tok", &calls))
	c.CachePath = ""

	if err := c.Authenticate(false); err != nil {
		t.Fatal(err)
	}
	if c.token != "tok" {
		t.Errorf("token = %q, want %q", c.token, "tok")
	}
}

func TestAuthenticateErrorStatus(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid api key", http.StatusUnauthorized)
	})

	if err := c.Authenticate(false); err == nil {
		t.Fatal("want an error for status 401")
	}
	if c.token != "" {
		t.Errorf("token = %q, the error body must not become the token", c.token)
	}
	if _, err := os.Stat(c.CachePath); err == nil {
		t.Error("an error response must not be cached")
	}
}

func TestTokenExpiry(t *testing.T) {
	exp := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	payload := base64.RawURLEncoding.EncodeToString(
		[]byte(fmt.Sprintf(`{"sub":"me","exp":%d}`, exp.Unix())))

	if got := tokenExpiry("header." + payload + ".signature"); !got.Equal(exp) {
		t.Errorf("tokenExpiry = %s, want %s", got, exp)
	}

	// Anything that isn't a JWT with an exp claim falls back to 24h.
	noExp := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"me"}`))
	for _, token := range []string{"not-a-jwt", "a.!!!.c", "a." + noExp + ".c"} {
		got := tokenExpiry(token)
		if d := time.Until(got); d < 23*time.Hour || d > 25*time.Hour {
			t.Errorf("tokenExpiry(%q) is %s from now, want about 24h", token, d)
		}
	}
}

const activitiesPath = "/activitylist-service/activities/search/activities"

func TestGetActivities(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != activitiesPath {
			t.Errorf("path = %q, want %q", r.URL.Path, activitiesPath)
		}
		q := r.URL.Query()
		if q.Get("start") != "5" || q.Get("limit") != "10" || q.Get("activityType") != "running" {
			t.Errorf("query = %q", r.URL.RawQuery)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer tok")
		}
		// A trimmed-down copy of a real response, with a field we don't use.
		fmt.Fprint(w, `[{
			"activityId": 123,
			"activityName": "Morning Run",
			"startTimeLocal": "2026-11-17 07:00:00",
			"activityType": {"typeId": 1, "typeKey": "running"},
			"distance": 10012.5,
			"duration": 3001.2,
			"averageSpeed": 3.336,
			"averageHR": 148,
			"aerobicTrainingEffect": 3.2,
			"someNewField": true
		}]`)
	})
	c.token = "tok"

	activities, err := c.getActivities(5, 10, "running")
	if err != nil {
		t.Fatal(err)
	}
	if len(activities) != 1 {
		t.Fatalf("got %d activities, want 1", len(activities))
	}
	a := activities[0]
	if a.ActivityID != 123 || a.Name != "Morning Run" || a.Type.TypeKey != "running" ||
		a.Distance != 10012.5 || a.Duration != 3001.2 || a.AverageHR != 148 || a.AerobicTE != 3.2 {
		t.Errorf("decoded %+v", a)
	}
}

func TestGetActivitiesOmitsEmptyType(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("activityType") {
			t.Errorf("query %q has activityType, want none", r.URL.RawQuery)
		}
		fmt.Fprint(w, `[]`)
	})
	if _, err := c.getActivities(0, 10, ""); err != nil {
		t.Fatal(err)
	}
}

func TestGetActivitiesErrors(t *testing.T) {
	tests := []struct {
		name             string
		status           int
		body             string
		wantUnauthorized bool
	}{
		{"401 is ErrUnauthorized", http.StatusUnauthorized, "", true},
		{"429 is another error", http.StatusTooManyRequests, "", false},
		{"500 is another error", http.StatusInternalServerError, "", false},
		{"bad JSON", http.StatusOK, `{"not": "a list"}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				fmt.Fprint(w, tt.body)
			})

			_, err := c.getActivities(0, 10, "")
			if err == nil {
				t.Fatal("want an error")
			}
			if got := errors.Is(err, ErrUnauthorized); got != tt.wantUnauthorized {
				t.Errorf("errors.Is(%v, ErrUnauthorized) = %v, want %v", err, got, tt.wantUnauthorized)
			}
		})
	}
}

// pagedServer fakes an account with total activities, one per day going
// back from newest, and serves them page by page like the real API.
func pagedServer(t *testing.T, total int, newest time.Time, requests *atomic.Int32) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		start, _ := strconv.Atoi(r.URL.Query().Get("start"))
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		page := []Activity{}
		for i := start; i < min(start+limit, total); i++ {
			var a Activity
			a.ActivityID = int64(i)
			a.StartTimeLocal = newest.AddDate(0, 0, -i).Format("2006-01-02 15:04:05")
			page = append(page, a)
		}
		if err := json.NewEncoder(w).Encode(page); err != nil {
			t.Error(err)
		}
	}
}

func TestFetchActivitiesPaging(t *testing.T) {
	newest := time.Date(2026, 11, 18, 7, 0, 0, 0, time.Local)
	tests := []struct {
		name         string
		total        int
		n            int
		since        time.Time
		wantCount    int
		wantRequests int32
	}{
		{"one page is enough", 200, 10, newest.AddDate(0, 0, -7), pageSize, 1},
		{"since needs a second page", 200, 10, newest.AddDate(0, 0, -60), 2 * pageSize, 2},
		{"n needs a second page", 200, 70, newest, 2 * pageSize, 2},
		{"stops at the last short page", 120, 1000, newest.AddDate(-5, 0, 0), 120, 3},
		{"no activities", 0, 10, newest, 0, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requests atomic.Int32
			c := newTestClient(t, pagedServer(t, tt.total, newest, &requests))

			activities, err := c.FetchActivities(tt.n, "", tt.since)
			if err != nil {
				t.Fatal(err)
			}
			if len(activities) != tt.wantCount {
				t.Errorf("got %d activities, want %d", len(activities), tt.wantCount)
			}
			if got := requests.Load(); got != tt.wantRequests {
				t.Errorf("made %d requests, want %d", got, tt.wantRequests)
			}
			for i, a := range activities {
				if a.ActivityID != int64(i) {
					t.Fatalf("activity %d has ID %d: pages overlap or skip", i, a.ActivityID)
				}
			}
		})
	}
}
