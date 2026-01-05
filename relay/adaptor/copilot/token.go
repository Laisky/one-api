package copilot

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"

	"github.com/songquanpeng/one-api/common/client"
)

const (
	githubCopilotTokenEndpoint = "https://api.github.com/copilot_internal/v2/token"
	tokenRefreshSkew           = 2 * time.Minute
)

type cachedToken struct {
	value     string
	expiresAt time.Time
}

var (
	tokenMu    sync.Mutex
	tokenCache = make(map[int]cachedToken)
)

// fetchCopilotTokenFunc fetches a Copilot API token from GitHub. It is a variable so tests
// can replace it.
var fetchCopilotTokenFunc = fetchCopilotToken

// GetCopilotAPIToken returns a cached Copilot API token for a channel, refreshing it when
// the token is expired (or close to expiry).
func GetCopilotAPIToken(ctx context.Context, channelID int, githubAccessToken string) (string, error) {
	if strings.TrimSpace(githubAccessToken) == "" {
		return "", errors.New("github access token is empty")
	}

	now := time.Now()
	tokenMu.Lock()
	cached, ok := tokenCache[channelID]
	if ok && cached.value != "" && cached.expiresAt.After(now.Add(tokenRefreshSkew)) {
		value := cached.value
		tokenMu.Unlock()
		return value, nil
	}
	tokenMu.Unlock()

	value, expiresAt, err := fetchCopilotTokenFunc(ctx, githubAccessToken)
	if err != nil {
		return "", errors.Wrap(err, "fetch copilot token")
	}
	if strings.TrimSpace(value) == "" {
		return "", errors.New("copilot token is empty")
	}
	if expiresAt.IsZero() {
		expiresAt = now.Add(20 * time.Minute)
	}

	tokenMu.Lock()
	tokenCache[channelID] = cachedToken{value: value, expiresAt: expiresAt}
	tokenMu.Unlock()
	return value, nil
}

// fetchCopilotToken exchanges a GitHub access token for a short-lived Copilot API token.
func fetchCopilotToken(ctx context.Context, githubAccessToken string) (token string, expiresAt time.Time, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubCopilotTokenEndpoint, nil)
	if err != nil {
		return "", time.Time{}, errors.Wrap(err, "new request")
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "token "+githubAccessToken)

	hc := client.ImpatientHTTPClient
	if hc == nil {
		hc = http.DefaultClient
	}

	resp, err := hc.Do(req)
	if err != nil {
		return "", time.Time{}, errors.Wrap(err, "do request")
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		return "", time.Time{}, errors.Wrap(readErr, "read response")
	}
	if resp.StatusCode != http.StatusOK {
		return "", time.Time{}, errors.Errorf("copilot token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	// The response shape is not formally documented; accept multiple variants.
	var raw struct {
		Token      string `json:"token"`
		ExpiresAt  any    `json:"expires_at"`
		ExpiresIn  any    `json:"expires_in"`
		RefreshIn  any    `json:"refresh_in"`
		Error      any    `json:"error"`
		ErrorDesc  any    `json:"error_description"`
		Message    any    `json:"message"`
		Telemetry  any    `json:"telemetry"`
		Endpoints  any    `json:"endpoints"`
		TokenType  any    `json:"token_type"`
		TokenScope any    `json:"scope"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return "", time.Time{}, errors.Wrap(err, "unmarshal token response")
	}

	if raw.Token == "" {
		return "", time.Time{}, errors.Errorf("missing token in response: %s", string(body))
	}

	now := time.Now()
	expiresAt, _ = parseExpiry(now, raw.ExpiresAt, raw.ExpiresIn, raw.RefreshIn)
	return raw.Token, expiresAt, nil
}

// parseExpiry derives expiry from potentially heterogeneous JSON fields.
func parseExpiry(now time.Time, expiresAtRaw any, expiresInRaw any, refreshInRaw any) (time.Time, error) {
	if t, err := parseTimeAny(expiresAtRaw); err == nil && !t.IsZero() {
		return t, nil
	}

	if d, ok := parseDurationSeconds(expiresInRaw); ok {
		return now.Add(d), nil
	}
	if d, ok := parseDurationSeconds(refreshInRaw); ok {
		return now.Add(d), nil
	}

	return time.Time{}, errors.New("unable to parse expiry")
}

// parseDurationSeconds parses a JSON value (string/number) interpreted as seconds.
func parseDurationSeconds(v any) (time.Duration, bool) {
	switch vv := v.(type) {
	case float64:
		if vv <= 0 {
			return 0, false
		}
		return time.Duration(vv) * time.Second, true
	case int64:
		if vv <= 0 {
			return 0, false
		}
		return time.Duration(vv) * time.Second, true
	case json.Number:
		asInt, err := vv.Int64()
		if err != nil || asInt <= 0 {
			return 0, false
		}
		return time.Duration(asInt) * time.Second, true
	case string:
		vv = strings.TrimSpace(vv)
		if vv == "" {
			return 0, false
		}
		asInt, err := strconv.ParseInt(vv, 10, 64)
		if err != nil || asInt <= 0 {
			return 0, false
		}
		return time.Duration(asInt) * time.Second, true
	default:
		return 0, false
	}
}

// parseTimeAny parses an RFC3339 timestamp or unix seconds/milliseconds from a JSON value.
func parseTimeAny(v any) (time.Time, error) {
	switch vv := v.(type) {
	case string:
		vv = strings.TrimSpace(vv)
		if vv == "" {
			return time.Time{}, errors.New("empty time string")
		}
		// Prefer RFC3339; some implementations return unix seconds as string.
		if t, err := time.Parse(time.RFC3339, vv); err == nil {
			return t, nil
		}
		if n, err := strconv.ParseInt(vv, 10, 64); err == nil {
			return unixSecondsOrMillis(n), nil
		}
		return time.Time{}, errors.Errorf("unsupported time format: %q", vv)
	case float64:
		return unixSecondsOrMillis(int64(vv)), nil
	case int64:
		return unixSecondsOrMillis(vv), nil
	case json.Number:
		n, err := vv.Int64()
		if err != nil {
			return time.Time{}, errors.Wrap(err, "parse json number")
		}
		return unixSecondsOrMillis(n), nil
	default:
		return time.Time{}, errors.Errorf("unsupported time type: %T", v)
	}
}

// unixSecondsOrMillis converts a unix timestamp expressed in seconds or milliseconds into time.
func unixSecondsOrMillis(n int64) time.Time {
	// Heuristic: > 1e12 is likely milliseconds.
	if n > 1_000_000_000_000 {
		return time.UnixMilli(n)
	}
	return time.Unix(n, 0)
}
