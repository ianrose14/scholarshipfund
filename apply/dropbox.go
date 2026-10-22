package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
)

// dropboxStorage implements FileStorage using the Dropbox HTTP API.
// https://www.dropbox.com/developers/documentation/http/documentation
//
// It authenticates with a long-lived refresh token (offline access), exchanging
// it for short-lived access tokens as needed.
type dropboxStorage struct {
	appKey, appSecret, refreshToken string
	client                          *http.Client

	mu          sync.Mutex
	accessToken string
	expires     time.Time
}

func newDropboxStorage(appKey, appSecret, refreshToken string) *dropboxStorage {
	return &dropboxStorage{
		appKey:       appKey,
		appSecret:    appSecret,
		refreshToken: refreshToken,
		client:       &http.Client{Timeout: 2 * time.Minute},
	}
}

func (d *dropboxStorage) token(ctx context.Context, forceRefresh bool) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !forceRefresh && d.accessToken != "" && time.Now().Before(d.expires) {
		return d.accessToken, nil
	}
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {d.refreshToken},
		"client_id":     {d.appKey},
		"client_secret": {d.appSecret},
	}
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.dropboxapi.com/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := d.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("dropbox token refresh: %s: %s", resp.Status, body)
	}
	var tr struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", err
	}
	d.accessToken = tr.AccessToken
	d.expires = time.Now().Add(time.Duration(tr.ExpiresIn)*time.Second - 5*time.Minute)
	return d.accessToken, nil
}

type dropboxError struct {
	status  int
	summary string
}

func (e *dropboxError) Error() string { return fmt.Sprintf("dropbox: %d %s", e.status, e.summary) }

// call makes a Dropbox API request. If arg is non-nil it is sent in the
// Dropbox-API-Arg header ("content" endpoints); otherwise jsonBody is sent as
// the request body ("rpc" endpoints).
func (d *dropboxStorage) call(ctx context.Context, endpoint string, arg any, body []byte, jsonBody any) (*http.Response, []byte, error) {
	for attempt := 0; ; attempt++ {
		tok, err := d.token(ctx, attempt > 0)
		if err != nil {
			return nil, nil, err
		}
		var rd io.Reader
		contentType := "application/octet-stream"
		if jsonBody != nil {
			b, _ := json.Marshal(jsonBody)
			rd, contentType = bytes.NewReader(b), "application/json"
		} else if body != nil {
			rd = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, "POST", endpoint, rd)
		if err != nil {
			return nil, nil, err
		}
		req.Header.Set("Authorization", "Bearer "+tok)
		if rd != nil {
			req.Header.Set("Content-Type", contentType)
		}
		if arg != nil {
			req.Header.Set("Dropbox-API-Arg", dropboxArg(arg))
		}
		resp, err := d.client.Do(req)
		if err != nil {
			return nil, nil, err
		}
		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, nil, err
		}
		switch {
		case resp.StatusCode == 401 && attempt == 0:
			continue // access token expired early; refresh and retry once
		case resp.StatusCode == 429 && attempt < 3:
			time.Sleep(time.Duration(attempt+1) * time.Second)
			continue
		case resp.StatusCode != 200:
			var e struct {
				Summary string `json:"error_summary"`
			}
			if json.Unmarshal(respBody, &e) != nil || e.Summary == "" {
				e.Summary = string(respBody)
			}
			return resp, nil, &dropboxError{resp.StatusCode, e.Summary}
		}
		return resp, respBody, nil
	}
}

// dropboxArg JSON-encodes v for the Dropbox-API-Arg header, which must be
// ASCII: non-ASCII characters are escaped as \uXXXX.
func dropboxArg(v any) string {
	b, _ := json.Marshal(v)
	var sb strings.Builder
	for _, r := range string(b) {
		switch {
		case r < 0x80:
			sb.WriteRune(r)
		case r > 0xFFFF:
			r1, r2 := utf16.EncodeRune(r)
			fmt.Fprintf(&sb, `\u%04x\u%04x`, r1, r2)
		default:
			fmt.Fprintf(&sb, `\u%04x`, r)
		}
	}
	return sb.String()
}

func isDropboxErr(err error, prefix string) bool {
	de, ok := err.(*dropboxError)
	return ok && de.status == 409 && strings.HasPrefix(de.summary, prefix)
}

func (d *dropboxStorage) Get(ctx context.Context, p string) ([]byte, string, error) {
	resp, body, err := d.call(ctx, "https://content.dropboxapi.com/2/files/download", map[string]string{"path": p}, nil, nil)
	if isDropboxErr(err, "path/not_found") {
		return nil, "", errNotFound
	}
	if err != nil {
		return nil, "", err
	}
	var meta struct {
		Rev string `json:"rev"`
	}
	if err := json.Unmarshal([]byte(resp.Header.Get("Dropbox-API-Result")), &meta); err != nil {
		return nil, "", fmt.Errorf("dropbox download %s: bad result header: %w", p, err)
	}
	return body, meta.Rev, nil
}

func (d *dropboxStorage) upload(ctx context.Context, p string, data []byte, mode any) error {
	arg := map[string]any{"path": p, "mode": mode, "autorename": false, "mute": true}
	_, _, err := d.call(ctx, "https://content.dropboxapi.com/2/files/upload", arg, data, nil)
	if isDropboxErr(err, "path/conflict") {
		return errConflict
	}
	return err
}

func (d *dropboxStorage) Put(ctx context.Context, p string, data []byte, ifRev string) error {
	if ifRev == "" {
		return d.upload(ctx, p, data, "overwrite")
	}
	return d.upload(ctx, p, data, map[string]string{".tag": "update", "update": ifRev})
}

func (d *dropboxStorage) Create(ctx context.Context, p string, data []byte) error {
	return d.upload(ctx, p, data, "add")
}

func (d *dropboxStorage) Delete(ctx context.Context, p string) error {
	_, _, err := d.call(ctx, "https://api.dropboxapi.com/2/files/delete_v2", nil, nil, map[string]string{"path": p})
	if isDropboxErr(err, "path_lookup/not_found") {
		return nil
	}
	return err
}

func (d *dropboxStorage) ListFolders(ctx context.Context, dir string) ([]string, error) {
	type listResult struct {
		Entries []struct {
			Tag  string `json:".tag"`
			Name string `json:"name"`
		} `json:"entries"`
		Cursor  string `json:"cursor"`
		HasMore bool   `json:"has_more"`
	}
	var out []string
	_, body, err := d.call(ctx, "https://api.dropboxapi.com/2/files/list_folder", nil, nil, map[string]any{"path": dir, "limit": 2000})
	for {
		if isDropboxErr(err, "path/not_found") {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		var lr listResult
		if err := json.Unmarshal(body, &lr); err != nil {
			return nil, err
		}
		for _, e := range lr.Entries {
			if e.Tag == "folder" {
				out = append(out, e.Name)
			}
		}
		if !lr.HasMore {
			return out, nil
		}
		_, body, err = d.call(ctx, "https://api.dropboxapi.com/2/files/list_folder/continue", nil, nil, map[string]string{"cursor": lr.Cursor})
	}
}
