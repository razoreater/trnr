# trnr plan

A small Go CLI that gives an overview of recent Garmin training, using the official
Garmin Connect **Personal API**. Stdlib only (Go 1.22+): `net/http`, `encoding/json`,
`flag`, `text/tabwriter`, `time`, `os`. The optional step 9 adds lipgloss for styling.

Tick a step when it's done (one commit per step).

## Garmin Personal API

- **Key:** create one at https://connect.garmin.com/app/settings/personalApiToken.
  It expires after 30/90/180/365 days and is shown only once. Keep it out of git.
- **Token:** exchange the key for an access token:
  ```bash
  curl -X POST 'https://personal.connectapi.garmin.com/personalapitoken-service/personal-api/personal-api-key/token' \
    -H "Authorization: ApiKey $GARMIN_API_KEY"
  ```
  The response is `text/plain`: the body is the token itself (a JWT starting with `eyJ`,
  about 1100 characters), not JSON.
  The token is valid for 24h. There is no refresh token, so request a new one when it expires.
- **Data calls:** same host, header `Authorization: Bearer <token>`.
- **Docs (Swagger):** https://developer.garmin.com/gc-developer-program/personal/swagger-docs/
- **FAQ:** https://developer.garmin.com/gc-developer-program/personal/personal-faq/

### Endpoints
| Purpose | Endpoint |
|---|---|
| Activity list | `GET /activitylist-service/activities/search/activities?start=0&limit=10` (also `activityType`, `sortBy`, `minDistance`, ...) |
| Activity detail | `GET /activity-service/activity/{activityId}` |
| HRV | `GET /hrv-service/hrv/daily/{fromDate}/{toDate}` |
| Sleep | `GET /sleep-service/sleep/dailySleepSummariesByDate` |
| Body battery | `GET /wellness-service/wellness/bodyBattery/reports/daily` |
| Daily summaries | `GET /usersummary-service/v2/dailysummaries` |
| Fitness age | `GET /fitnessage-service/fitnessage/v2/{calendarDate}` |

Activity list fields you'll need: `startTimeLocal`, `activityName`, `activityType`
(object with `typeKey`), `distance` (m), `duration` (s), `averageHR`, `averageSpeed` (m/s),
`calories`, `elevationGain`, `aerobicTrainingEffect`, `activityTrainingLoad`.

## Steps

### [x] 1. Skeleton
- [x] `go mod init github.com/razoreater/trnr`
- [x] `main.go` that prints a welcome text
- [x] Read `GARMIN_API_KEY` with `os.Getenv`. If it's empty, print to stderr
  (`fmt.Fprintln(os.Stderr, ...)`) and `os.Exit(1)`. Otherwise print the key's length, never the key.
- [x] `.gitignore`: the `trnr` binary and `.env`
- Concepts: `package main`, zero values (`""` instead of None), `:=`, unused imports
  are compile errors, `go run .`, `go build`, `gofmt -l .`

### [x] 2. Get an access token
- `func getToken(apiKey string) (string, error)`
- [x] Run the curl above by hand to see the response shape (plain text, not JSON).
- [x] POST with no body, read it with `io.ReadAll`, and return `strings.TrimSpace(string(body))`.
  Check `resp.StatusCode` first, so an error body is never treated as a token.
- Concepts: `http.NewRequest`, setting headers, `defer resp.Body.Close()`, checking
  `resp.StatusCode`, `io.ReadAll`, creating and wrapping errors with `fmt.Errorf` (`%w`),
  and the `(value, error)` return pattern with `if err != nil`.
- Done when `go run .` prints "token OK". Never print the token.

### [x] 3. Fetch activities
- A `client` struct (`*http.Client`, token, base URL) with method
  `getActivities(limit int) ([]Activity, error)`.
- An `Activity` struct with only the fields you need. Unknown JSON fields are ignored.
- First check the real response shape by hand (pipe the curl through `jq`).
- Concepts: methods and receivers, slices, `url.Values` for query strings, nested structs,
  `json.NewDecoder(...).Decode(&v)` into a struct with `json:"..."` tags.

### [x] 4. Print a table
- `text/tabwriter` columns: date | type | name | km | h:mm | pace or speed | avg HR | TE
- Show pace (min/km) for running and km/h for cycling, using a `switch` on `typeKey`.
- Concepts: `time.Parse`, `time.Duration`, `fmt` verbs (`%5.1f`, `%-20s`).
- Done when the table matches Garmin Connect web (spot-check distance and time).

### [x] 5. Flags and weekly totals
- Flags `-n 10`, `-type running`, `-weeks 4` via the `flag` package.
- Group by ISO week (`t.ISOWeek()`) into a `map[string]*weekTotal`, sort the keys, and print
  distance, time and count per week.
- Concepts: maps, pointers, `sort.Slice` / `slices.Sort`.

### [ ] 6. Token cache
- Save the token and its expiry to `~/.cache/trnr/token.json` (`os.UserCacheDir`)
  with file mode `0600`. Reuse it while it's valid and fetch a new one on expiry or a 401.
- The token response has no expiry field: use `time.Now().Add(24 * time.Hour)`, or
  (extra) decode the JWT's `exp` claim.
- Concepts: file I/O, `os.MkdirAll`, `json.Marshal`, `time.Now().Before(...)`.

### [ ] 7. Wellness (optional)
- A `-wellness` flag that shows HRV, sleep and body battery for the last 7 days.

### [ ] 8. Package split and tests (optional)
- Move the API client to `internal/garmin/`.
- Table-driven tests for formatting, plus `httptest.Server` to fake the API.

### [ ] 9. Styled output with lipgloss (optional)
- The first third-party dependency: `go get github.com/charmbracelet/lipgloss`.
- Replace the `text/tabwriter` table with `lipgloss/table`: rounded border, bold header,
  right-aligned number columns via `StyleFunc(func(row, col int) lipgloss.Style)`.
- Color rows by `typeKey` (e.g. running vs cycling).
- Show the weekly totals from step 5 in a bordered summary box above the table.
- Keep the cell formatting (km, h:mm, pace) from step 4. Only the rendering changes.
- Check that `trnr | less` still prints clean text (lipgloss drops colors when stdout
  isn't a terminal).
- Concepts: `go get`, `go.mod` / `go.sum`, importing a subpackage, method chaining,
  passing functions as values.

## Target layout
- `main.go`: flags, wiring, output
- `garmin.go`: token and API client (later `internal/garmin/`)
- `report.go`: table and weekly aggregation

## Checks
- `go vet ./...` and `gofmt -l .` are clean, and `go test ./...` passes from step 8.
- 401 means the token or key expired. 429 means slow down. Keep an eye on the key's expiry date.
