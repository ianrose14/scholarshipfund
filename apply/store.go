package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	errNotFound = errors.New("not found")
	errConflict = errors.New("conflict")
)

// FileStorage is the minimal file API we need. Paths are absolute and
// slash-separated (Dropbox style), e.g. "/2027/jane@example.com/applicant.json".
type FileStorage interface {
	// Get returns the file contents and an opaque revision identifier.
	Get(ctx context.Context, p string) (data []byte, rev string, err error)
	// Put writes a file. If ifRev is empty the file is unconditionally
	// overwritten (or created); otherwise the write fails with errConflict
	// unless the current revision equals ifRev.
	Put(ctx context.Context, p string, data []byte, ifRev string) error
	// Create writes a new file, failing with errConflict if it already exists.
	Create(ctx context.Context, p string, data []byte) error
	// Delete removes a file; deleting a missing file is not an error.
	Delete(ctx context.Context, p string) error
	// ListFolders returns the names of the immediate subfolders of dir.
	ListFolders(ctx context.Context, dir string) ([]string, error)
}

// Store keeps one folder per applicant under root, named after their email.
type Store struct {
	fs   FileStorage
	root string
}

var unsafeFolderChars = regexp.MustCompile(`[^a-z0-9@._+-]`)

func (s *Store) dir(email string) string {
	return path.Join(s.root, unsafeFolderChars.ReplaceAllString(normalizeEmail(email), "_"))
}

func (s *Store) Load(ctx context.Context, email string) (*Applicant, string, error) {
	data, rev, err := s.fs.Get(ctx, path.Join(s.dir(email), applicantJSONFile))
	if err != nil {
		return nil, "", err
	}
	var a Applicant
	if err := json.Unmarshal(data, &a); err != nil {
		return nil, "", fmt.Errorf("parsing %s for %s: %w", applicantJSONFile, email, err)
	}
	return &a, rev, nil
}

// Ensure loads the applicant, creating a new record if none exists.
func (s *Store) Ensure(ctx context.Context, email string, now time.Time) (*Applicant, error) {
	a, _, err := s.Load(ctx, email)
	if !errors.Is(err, errNotFound) {
		return a, err
	}
	a = &Applicant{Email: normalizeEmail(email), CreatedAt: now, UpdatedAt: now, Recommenders: []*Recommender{}}
	data, _ := json.MarshalIndent(a, "", "  ")
	err = s.fs.Create(ctx, path.Join(s.dir(email), applicantJSONFile), data)
	if errors.Is(err, errConflict) { // created concurrently
		a, _, err = s.Load(ctx, email)
	}
	return a, err
}

// Update performs a read-modify-write of applicant.json, retrying if someone
// else (e.g. a recommender uploading at the same moment) wrote in between.
// fn may be called more than once, so it must only modify a.
func (s *Store) Update(ctx context.Context, email string, fn func(a *Applicant) error) (*Applicant, error) {
	for attempt := 0; attempt < 5; attempt++ {
		a, rev, err := s.Load(ctx, email)
		if err != nil {
			return nil, err
		}
		if err := fn(a); err != nil {
			return nil, err
		}
		data, _ := json.MarshalIndent(a, "", "  ")
		err = s.fs.Put(ctx, path.Join(s.dir(email), applicantJSONFile), data, rev)
		if errors.Is(err, errConflict) {
			slog.Warn("applicant.json write conflict; retrying", "email", email, "attempt", attempt)
			time.Sleep(time.Duration(50*(attempt+1)) * time.Millisecond)
			continue
		}
		return a, err
	}
	return nil, fmt.Errorf("updating %s: too many conflicting writes", email)
}

func (s *Store) PutFile(ctx context.Context, email, name string, data []byte) error {
	return s.fs.Put(ctx, path.Join(s.dir(email), name), data, "")
}

func (s *Store) DeleteFile(ctx context.Context, email, name string) error {
	return s.fs.Delete(ctx, path.Join(s.dir(email), name))
}

// ListApplicants loads every applicant record, sorted by last name.
func (s *Store) ListApplicants(ctx context.Context) ([]*Applicant, error) {
	folders, err := s.fs.ListFolders(ctx, s.root)
	if err != nil {
		return nil, err
	}
	var (
		mu   sync.Mutex
		wg   sync.WaitGroup
		out  []*Applicant
		errs []error
		sem  = make(chan struct{}, 8)
	)
	for _, f := range folders {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			data, _, err := s.fs.Get(ctx, path.Join(s.root, f, applicantJSONFile))
			if errors.Is(err, errNotFound) {
				return // some other folder; ignore
			}
			var a Applicant
			if err == nil {
				err = json.Unmarshal(data, &a)
			}
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", f, err))
				return
			}
			out = append(out, &a)
		}()
	}
	wg.Wait()
	sort.Slice(out, func(i, j int) bool {
		ki := strings.ToLower(out[i].Form.LastName + " " + out[i].Form.FirstName + " " + out[i].Email)
		kj := strings.ToLower(out[j].Form.LastName + " " + out[j].Form.FirstName + " " + out[j].Email)
		return ki < kj
	})
	return out, errors.Join(errs...)
}

var unsafeFilenameChars = regexp.MustCompile(`[/\\:*?"<>|\x00-\x1f]+`)

// safeFilename makes a user-supplied string usable as (part of) a file name.
func safeFilename(s string) string {
	s = strings.TrimSpace(unsafeFilenameChars.ReplaceAllString(s, " "))
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 80 {
		s = string(r[:80])
	}
	return strings.Trim(s, ". ")
}
