package garmin

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const tokenPath = "/personalapitoken-service/personal-api/personal-api-key/token"

// cachedToken is what we store in the token cache file.
type cachedToken struct {
	Token   string    `json:"token"`
	Expires time.Time `json:"expires"`
}

// DefaultCachePath returns ~/.cache/trnr/token.json (or the OS equivalent).
func DefaultCachePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("find cache dir: %w", err)
	}
	return filepath.Join(dir, "trnr", "token.json"), nil
}

// requestToken exchanges the API key for a new access token.
func (c *Client) requestToken() (string, error) {
	request, err := http.NewRequest(http.MethodPost, c.BaseURL+tokenPath, nil)
	if err != nil {
		return "", fmt.Errorf("build token request: %w", err)
	}
	request.Header.Set("Authorization", "ApiKey "+c.APIKey)

	response, err := c.HTTP.Do(request)
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

// loadCachedToken returns the cached token if there is one that is still
// valid for at least a few minutes. Any problem just means "no token".
func loadCachedToken(path string) (string, bool) {
	if path == "" {
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

func saveToken(path, token string, expires time.Time) error {
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

// Authenticate gives the client an access token. It reuses the cached token
// when possible. With refresh set, or without a valid cached token, it
// requests a new one and caches it.
func (c *Client) Authenticate(refresh bool) error {
	if !refresh {
		if token, ok := loadCachedToken(c.CachePath); ok {
			c.token = token
			return nil
		}
	}
	token, err := c.requestToken()
	if err != nil {
		return err
	}
	c.token = token
	if c.CachePath != "" {
		if err := saveToken(c.CachePath, token, tokenExpiry(token)); err != nil {
			// Not fatal: we have a token, it just won't be reused next time.
			fmt.Fprintln(os.Stderr, "warning: cache token:", err)
		}
	}
	return nil
}
