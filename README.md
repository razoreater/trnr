# trnr
Trainer gives a simple CLI overview of recent Garmin activities

## Setup

1. Create a Personal API key at
   https://connect.garmin.com/app/settings/personalApiToken.
2. Copy `.env.example` to `.env` and put your key in it:
   ```bash
   cp .env.example .env
   ```
3. Load `.env` into your shell. Run this from the project root, once per terminal
   session, and again after editing `.env`:
   ```bash
   set -a; source .env; set +a
   ```
   `set -a` exports every variable that `.env` sets, so `go run .` can read them.
   Check that it worked without showing the key:
   ```bash
   echo ${#GARMIN_API_KEY}
   ```

## Run

```bash
go run .
```
