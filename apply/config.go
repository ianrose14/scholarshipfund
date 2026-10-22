package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// Config is loaded entirely from environment variables (see README.md).
type Config struct {
	Port    string
	BaseURL string // e.g. https://apply.allisonrosememorialfund.org (no trailing slash)

	SecretKey []byte // HMAC key for all signed tokens

	Cycle          string    // e.g. "2027"; shown to users
	Deadline       time.Time // applicant edits are locked after this
	LetterDeadline time.Time // recommenders may upload until this (defaults to Deadline)

	AdminEmails map[string]bool

	// Dropbox (production). If DropboxRefreshToken is empty, LocalDataDir is used instead.
	DropboxAppKey       string
	DropboxAppSecret    string
	DropboxRefreshToken string
	StorageRoot         string // folder (in Dropbox or LocalDataDir) holding one subfolder per applicant
	LocalDataDir        string

	// Resend (production). If ResendAPIKey is empty, emails are printed to the log.
	ResendAPIKey string
	EmailFrom    string
	EmailReplyTo string
	NotifyEmails []string // who gets "application complete" notices; defaults to AdminEmails

	CronSecret string // shared secret required by /tasks/* endpoints
}

func (c Config) secureCookies() bool { return strings.HasPrefix(c.BaseURL, "https://") }

func loadConfig() (Config, error) {
	var errs []error
	get := func(key, def string) string {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
		return def
	}
	require := func(key string) string {
		v := get(key, "")
		if v == "" {
			errs = append(errs, fmt.Errorf("%s is required", key))
		}
		return v
	}
	parseTime := func(key, v string) time.Time {
		if v == "" {
			return time.Time{}
		}
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: must be RFC3339 (e.g. 2027-03-01T23:59:59-05:00): %w", key, err))
		}
		return t
	}

	c := Config{
		Port:                get("PORT", "8080"),
		BaseURL:             strings.TrimRight(require("BASE_URL"), "/"),
		SecretKey:           []byte(require("SECRET_KEY")),
		Cycle:               require("CYCLE"),
		DropboxAppKey:       get("DROPBOX_APP_KEY", ""),
		DropboxAppSecret:    get("DROPBOX_APP_SECRET", ""),
		DropboxRefreshToken: get("DROPBOX_REFRESH_TOKEN", ""),
		LocalDataDir:        get("LOCAL_DATA_DIR", ""),
		ResendAPIKey:        get("RESEND_API_KEY", ""),
		EmailFrom:           get("EMAIL_FROM", "Dr. Allison Rose Memorial Fund <applications@allisonrosememorialfund.org>"),
		EmailReplyTo:        get("EMAIL_REPLY_TO", "applications@allisonrosememorialfund.org"),
		CronSecret:          get("CRON_SECRET", ""),
		AdminEmails:         map[string]bool{},
	}
	c.StorageRoot = "/" + strings.Trim(get("STORAGE_ROOT", "/"+c.Cycle), "/")
	c.Deadline = parseTime("DEADLINE", require("DEADLINE"))
	c.LetterDeadline = parseTime("LETTER_DEADLINE", get("LETTER_DEADLINE", ""))
	if c.LetterDeadline.IsZero() {
		c.LetterDeadline = c.Deadline
	}

	if len(c.SecretKey) > 0 && len(c.SecretKey) < 32 {
		errs = append(errs, errors.New("SECRET_KEY must be at least 32 characters"))
	}
	for _, e := range splitList(get("ADMIN_EMAILS", "")) {
		c.AdminEmails[normalizeEmail(e)] = true
	}
	c.NotifyEmails = splitList(get("NOTIFY_EMAILS", ""))
	if len(c.NotifyEmails) == 0 {
		for e := range c.AdminEmails {
			c.NotifyEmails = append(c.NotifyEmails, e)
		}
	}
	if c.DropboxRefreshToken != "" {
		if c.DropboxAppKey == "" || c.DropboxAppSecret == "" {
			errs = append(errs, errors.New("DROPBOX_APP_KEY and DROPBOX_APP_SECRET are required with DROPBOX_REFRESH_TOKEN"))
		}
	} else if c.LocalDataDir == "" {
		errs = append(errs, errors.New("set either DROPBOX_REFRESH_TOKEN (production) or LOCAL_DATA_DIR (development)"))
	}
	return c, errors.Join(errs...)
}

func splitList(s string) []string {
	var out []string
	for p := range strings.SplitSeq(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func normalizeEmail(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
