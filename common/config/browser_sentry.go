package config

import (
	"encoding/json"
	"io"
	"net/url"
	"regexp"
	"strings"

	"github.com/Laisky/errors/v2"
)

// BrowserSentryConfig contains only approved public browser error-reporting settings.
type BrowserSentryConfig struct {
	DSN         string `json:"dsn"`
	Environment string `json:"environment"`
}

var browserDSNKey = regexp.MustCompile(`^[a-f0-9]{32}$`)
var browserDSNHost = regexp.MustCompile(`^o[0-9]+\.ingest(?:\.[a-z]+)?\.sentry\.io$`)
var browserDSNProject = regexp.MustCompile(`^/[0-9]{1,20}$`)

// ParseBrowserSentryConfig validates one bounded public JSON setting, returning nil for an empty setting and a fixed error for invalid input.
func ParseBrowserSentryConfig(raw string) (*BrowserSentryConfig, error) {
	invalid := func() (*BrowserSentryConfig, error) {
		return nil, errors.New("invalid browser Sentry configuration; expected public dsn and environment only")
	}
	if len(raw) > 1024 {
		return invalid()
	}
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return invalid()
	}
	fields := make(map[string]string, 2)
	for decoder.More() {
		token, err = decoder.Token()
		if err != nil {
			return invalid()
		}
		key, ok := token.(string)
		if !ok || (key != "dsn" && key != "environment") {
			return invalid()
		}
		if _, exists := fields[key]; exists {
			return invalid()
		}
		var value string
		if err = decoder.Decode(&value); err != nil {
			return invalid()
		}
		fields[key] = value
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') {
		return invalid()
	}
	if _, err = decoder.Token(); err != io.EOF {
		return invalid()
	}
	if len(fields) != 2 {
		return invalid()
	}
	environment := fields["environment"]
	if environment != "production" && environment != "staging" && environment != "development" {
		return invalid()
	}
	if len(fields["dsn"]) > 256 {
		return invalid()
	}
	dsn, err := url.Parse(fields["dsn"])
	if err != nil || dsn == nil || dsn.User == nil {
		return invalid()
	}
	_, password := dsn.User.Password()
	if dsn.Scheme != "https" || password || dsn.Port() != "" || dsn.RawQuery != "" || dsn.ForceQuery || dsn.Fragment != "" || dsn.RawFragment != "" || dsn.RawPath != "" ||
		!browserDSNKey.MatchString(dsn.User.Username()) || !browserDSNHost.MatchString(dsn.Host) || !browserDSNProject.MatchString(dsn.Path) {
		return invalid()
	}
	return &BrowserSentryConfig{DSN: dsn.String(), Environment: environment}, nil
}
