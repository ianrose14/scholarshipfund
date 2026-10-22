package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

type server struct {
	cfg     Config
	store   *Store
	mail    Mailer
	tokens  signer
	limiter *rateLimiter
	pages   map[string]*template.Template
	now     func() time.Time
}

func newServer(cfg Config, store *Store, mail Mailer) *server {
	s := &server{
		cfg:     cfg,
		store:   store,
		mail:    mail,
		tokens:  signer{key: cfg.SecretKey},
		limiter: newRateLimiter(),
		pages:   map[string]*template.Template{},
		now:     time.Now,
	}
	funcs := template.FuncMap{
		"date":     func(t time.Time) string { return t.In(eastern).Format("Jan 2, 2006") },
		"datetime": func(t time.Time) string { return t.In(eastern).Format("Jan 2, 2006 3:04 PM MST") },
		"deadline": formatDeadline,
		"size":     humanSize,
		"dict": func(kv ...any) map[string]any {
			m := map[string]any{}
			for i := 0; i+1 < len(kv); i += 2 {
				m[kv[i].(string)] = kv[i+1]
			}
			return m
		},
	}
	pages, _ := fs.Glob(templateFS, "templates/*.html")
	for _, p := range pages {
		name := strings.TrimSuffix(strings.TrimPrefix(p, "templates/"), ".html")
		if name == "layout" {
			continue
		}
		s.pages[name] = template.Must(template.New("").Funcs(funcs).ParseFS(templateFS, "templates/layout.html", p))
	}
	return s
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })

	// sign in
	mux.HandleFunc("GET /{$}", s.handleHome)
	mux.HandleFunc("POST /login", s.handleLogin)
	mux.HandleFunc("GET /auth", s.handleAuthConfirm)
	mux.HandleFunc("POST /auth", s.handleAuth)
	mux.HandleFunc("POST /logout", s.handleLogout)

	// applicant
	mux.HandleFunc("GET /app", s.applicant(s.handleDashboard))
	mux.HandleFunc("GET /app/form", s.applicant(s.handleFormPage))
	mux.HandleFunc("POST /app/form", s.applicant(s.open(s.handleFormSave)))
	mux.HandleFunc("POST /app/upload/{kind}", s.applicant(s.open(s.handleUpload)))
	mux.HandleFunc("POST /app/recommenders", s.applicant(s.open(s.handleAddRecommender)))
	mux.HandleFunc("POST /app/recommenders/{id}", s.applicant(s.open(s.handleEditRecommender)))
	mux.HandleFunc("POST /app/recommenders/{id}/delete", s.applicant(s.open(s.handleDeleteRecommender)))
	mux.HandleFunc("POST /app/recommenders/{id}/resend", s.applicant(s.open(s.handleResendRecommender)))

	// recommender (authenticated by the token in the URL)
	mux.HandleFunc("GET /r/{token}", s.handleRecommenderPage)
	mux.HandleFunc("POST /r/{token}/upload", s.handleRecommenderUpload)
	mux.HandleFunc("POST /r/{token}/decline", s.handleRecommenderDecline)

	// admin
	mux.HandleFunc("GET /admin", s.admin(s.handleAdmin))
	mux.HandleFunc("GET /admin/applicants.csv", s.admin(s.handleAdminCSV))

	// called daily by Cloud Scheduler
	mux.HandleFunc("POST /tasks/reminders", s.handleReminders)

	return securityHeaders(http.NewCrossOriginProtection().Handler(mux))
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		// Our URLs carry tokens (/auth?t=..., /r/...), so never leak them via Referer.
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Content-Security-Policy", "default-src 'self'; style-src 'self' https://fonts.googleapis.com; font-src https://fonts.gstatic.com; img-src 'self' data:; form-action 'self'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

// ---- sessions ----

const sessionCookie = "session"

func (s *server) sessionEmail(r *http.Request) string {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return ""
	}
	cl, err := s.tokens.verify(c.Value, kindSession, s.now())
	if err != nil {
		return ""
	}
	return cl.Email
}

func (s *server) setSession(w http.ResponseWriter, email string) {
	tok := s.tokens.sign(claims{Kind: kindSession, Email: email, Exp: s.now().Add(sessionTTL).Unix()})
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: tok, Path: "/", MaxAge: int(sessionTTL.Seconds()),
		HttpOnly: true, Secure: s.cfg.secureCookies(), SameSite: http.SameSiteLaxMode,
	})
}

func (s *server) clearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: s.cfg.secureCookies(), SameSite: http.SameSiteLaxMode})
}

func (s *server) isAdmin(email string) bool { return s.cfg.AdminEmails[normalizeEmail(email)] }

type applicantHandler func(w http.ResponseWriter, r *http.Request, a *Applicant)

// applicant requires a signed-in applicant and loads their record.
func (s *server) applicant(h applicantHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		email := s.sessionEmail(r)
		if email == "" {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		a, _, err := s.store.Load(r.Context(), email)
		if errors.Is(err, errNotFound) {
			// session from before the folder was deleted, or a new applicant after the deadline
			s.clearSession(w)
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		h(w, r, a)
	}
}

// open rejects changes once the application deadline has passed.
func (s *server) open(h applicantHandler) applicantHandler {
	return func(w http.ResponseWriter, r *http.Request, a *Applicant) {
		if !s.isOpen() {
			s.redirectFlash(w, r, "/app", "closed")
			return
		}
		h(w, r, a)
	}
}

func (s *server) admin(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.isAdmin(s.sessionEmail(r)) {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		h(w, r)
	}
}

func (s *server) isOpen() bool { return s.now().Before(s.cfg.Deadline) }

// ---- rendering ----

type pageData struct {
	Title    string
	Email    string // signed-in user, if any
	IsAdmin  bool
	Flash    string
	Error    string
	Cycle    string
	Deadline time.Time
	Open     bool // Deadline hasn't passed
	NoNav    bool // hide the signed-in navigation

	// page-specific
	A            *Applicant
	Form         ApplicationForm
	Certify      bool
	SignName     string
	Statuses     []option
	Enrollments  []option
	Rec          *Recommender
	Token        string
	NewApplicant bool
	Applicants   []*Applicant
	Message      string
	MaxRecs      int
	LettersReq   int
}

var flashMessages = map[string]string{
	"saved":               "Your application form has been saved.",
	"uploaded":            "Your file has been uploaded.",
	"rec-added":           "Recommender added. We've emailed them a link to upload their letter.",
	"rec-updated":         "Recommender updated.",
	"rec-updated-emailed": "Recommender updated. We've emailed a new upload link to the new address (the old link no longer works).",
	"rec-removed":         "Recommender removed.",
	"rec-resent":          "We've re-sent the request email.",
	"closed":              "The application deadline has passed, so your application can no longer be changed.",
	"letter-thanks":       "Thank you! Your letter has been received.",
	"declined":            "Thank you for letting us know. We've notified the applicant.",
}

func (s *server) render(w http.ResponseWriter, r *http.Request, page string, status int, d pageData) {
	t, ok := s.pages[page]
	if !ok {
		s.serverError(w, r, errors.New("unknown page "+page))
		return
	}
	if !d.NoNav && d.Email == "" {
		d.Email = s.sessionEmail(r)
	}
	d.IsAdmin = s.isAdmin(d.Email)
	if d.Flash == "" {
		d.Flash = flashMessages[r.URL.Query().Get("msg")]
	}
	d.Cycle = s.cfg.Cycle
	if d.Deadline.IsZero() {
		d.Deadline = s.cfg.Deadline
	}
	d.Open = s.now().Before(d.Deadline)
	d.MaxRecs = maxRecommenders
	d.LettersReq = lettersRequired
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "layout", d); err != nil {
		s.serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	buf.WriteTo(w)
}

func (s *server) message(w http.ResponseWriter, r *http.Request, status int, title, msg string) {
	s.render(w, r, "message", status, pageData{Title: title, Message: msg})
}

func (s *server) serverError(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte("Sorry, something went wrong on our end. Please try again, or email applications@allisonrosememorialfund.org if the problem continues."))
}

func (s *server) redirectFlash(w http.ResponseWriter, r *http.Request, path, msg string) {
	http.Redirect(w, r, path+"?msg="+msg, http.StatusSeeOther)
}

// ---- updates ----

// userError is a problem the user can fix; its text is shown to them.
type userError string

func (e userError) Error() string { return string(e) }

// update modifies an applicant's record and, if that made the application
// complete for the first time, sends the "complete" emails.
func (s *server) update(ctx context.Context, email string, fn func(a *Applicant) error) (*Applicant, error) {
	var justCompleted bool
	a, err := s.store.Update(ctx, email, func(a *Applicant) error {
		justCompleted = false
		if err := fn(a); err != nil {
			return err
		}
		now := s.now()
		a.UpdatedAt = now
		if a.CompletedAt == nil && a.Complete() {
			a.CompletedAt = &now
			justCompleted = true
		}
		return nil
	})
	if err == nil && justCompleted {
		slog.Info("application complete", "email", email)
		s.send(ctx, applicationCompleteEmail(a))
		if len(s.cfg.NotifyEmails) > 0 {
			s.send(ctx, adminCompleteEmail(s.cfg.NotifyEmails, a, s.cfg.BaseURL+"/admin"))
		}
	}
	return a, err
}

// send sends an email, logging (rather than failing the request) on error.
func (s *server) send(ctx context.Context, m Email) error {
	err := s.mail.Send(ctx, m)
	if err != nil {
		slog.Error("sending email failed", "to", m.To, "subject", m.Subject, "err", err)
	}
	return err
}

// ---- misc helpers ----

func clientIP(r *http.Request) string {
	// Cloud Run's front end appends the real client address to X-Forwarded-For.
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[len(parts)-1])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func newID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%d KB", n>>10)
	default:
		return fmt.Sprintf("%d bytes", n)
	}
}
