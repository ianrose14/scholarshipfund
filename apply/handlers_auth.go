package main

import (
	"errors"
	"log/slog"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"
)

func (s *server) handleHome(w http.ResponseWriter, r *http.Request) {
	if email := s.sessionEmail(r); email != "" {
		if s.isAdmin(email) {
			http.Redirect(w, r, "/admin", http.StatusSeeOther)
		} else {
			http.Redirect(w, r, "/app", http.StatusSeeOther)
		}
		return
	}
	s.render(w, r, "login", http.StatusOK, pageData{Title: "Sign in"})
}

func validEmail(s string) bool {
	if len(s) > 254 || strings.ContainsAny(s, " <>") {
		return false
	}
	a, err := mail.ParseAddress(s)
	return err == nil && a.Address == s && strings.Contains(s[strings.LastIndex(s, "@"):], ".")
}

func (s *server) handleLogin(w http.ResponseWriter, r *http.Request) {
	email := normalizeEmail(r.FormValue("email"))
	if !validEmail(email) {
		s.render(w, r, "login", http.StatusBadRequest, pageData{Title: "Sign in", Error: "Please enter a valid email address."})
		return
	}
	now := s.now()
	if !s.limiter.allow("login-ip:"+clientIP(r), 20, time.Hour, now) {
		s.render(w, r, "login", http.StatusTooManyRequests, pageData{Title: "Sign in", Error: "Too many sign-in attempts. Please wait a while and try again."})
		return
	}
	// Same response whether or not we actually send, so this form can't be
	// used to spam someone or to discover who has applied.
	if s.limiter.allow("login-email:"+email, 3, 15*time.Minute, now) {
		tok := s.tokens.sign(claims{Kind: kindLogin, Email: email, Exp: now.Add(loginTTL).Unix()})
		s.send(r.Context(), loginEmail(email, s.cfg.BaseURL+"/auth?t="+url.QueryEscape(tok)))
	} else {
		slog.Warn("login email rate limited", "email", email)
	}
	s.render(w, r, "login_sent", http.StatusOK, pageData{Title: "Check your email", Message: email})
}

// handleAuthConfirm shows a "Continue" button rather than signing in directly:
// corporate/hospital email scanners often follow links in emails, and a GET
// must not be what establishes the session.
func (s *server) handleAuthConfirm(w http.ResponseWriter, r *http.Request) {
	tok := r.URL.Query().Get("t")
	c, err := s.tokens.verify(tok, kindLogin, s.now())
	if err != nil {
		s.render(w, r, "login", http.StatusBadRequest, pageData{Title: "Sign in", Error: "That sign-in link is invalid or has expired. Please request a new one."})
		return
	}
	// The applicant record isn't created until they click through, so a
	// missing record means this is their first sign-in.
	isNew := false
	if !s.isAdmin(c.Email) {
		_, _, err := s.store.Load(r.Context(), c.Email)
		isNew = errors.Is(err, errNotFound)
	}
	s.render(w, r, "auth_confirm", http.StatusOK, pageData{Title: "Sign in", Token: tok, NewApplicant: isNew})
}

func (s *server) handleAuth(w http.ResponseWriter, r *http.Request) {
	c, err := s.tokens.verify(r.FormValue("t"), kindLogin, s.now())
	if err != nil {
		s.render(w, r, "login", http.StatusBadRequest, pageData{Title: "Sign in", Error: "That sign-in link is invalid or has expired. Please request a new one."})
		return
	}
	if s.isAdmin(c.Email) {
		s.setSession(w, c.Email)
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	if s.isOpen() {
		_, err = s.store.Ensure(r.Context(), c.Email, s.now())
	} else {
		_, _, err = s.store.Load(r.Context(), c.Email)
		if errors.Is(err, errNotFound) {
			s.message(w, r, http.StatusOK, "Applications are closed",
				"The deadline for "+s.cfg.Cycle+" applications has passed. Please check back on our website for information about the next application cycle.")
			return
		}
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.setSession(w, c.Email)
	http.Redirect(w, r, "/app", http.StatusSeeOther)
}

func (s *server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.clearSession(w)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
