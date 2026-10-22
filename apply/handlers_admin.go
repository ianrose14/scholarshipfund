package main

import (
	"crypto/subtle"
	"encoding/csv"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *server) handleAdmin(w http.ResponseWriter, r *http.Request) {
	apps, err := s.store.ListApplicants(r.Context())
	if err != nil {
		if apps == nil {
			s.serverError(w, r, err)
			return
		}
		slog.Error("some applicant records could not be loaded", "err", err)
	}
	s.render(w, r, "admin", http.StatusOK, pageData{Title: "Applicants", Applicants: apps})
}

func (s *server) handleAdminCSV(w http.ResponseWriter, r *http.Request) {
	apps, err := s.store.ListApplicants(r.Context())
	if err != nil && apps == nil {
		s.serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="applicants-`+s.cfg.Cycle+`.csv"`)
	cw := csv.NewWriter(w)
	cw.Write([]string{
		"Last name", "First name", "Email", "Phone", "Address", "Address 2", "City", "State", "Zip",
		"Program", "Program status", "Status explanation", "Start date", "Anticipated graduation date", "Enrollment",
		"Planned credit hours (part-time)",
		"Form signed", "Personal statement", "Resume", "Financial aid", "Transcript", "Letters received", "Supervisor letter",
		"Recommenders", "Complete", "Completed at", "Created at", "Last updated",
	})
	yes := func(b bool) string {
		if b {
			return "yes"
		}
		return "no"
	}
	transcript := func(a *Applicant) string {
		if a.Transcript == nil && !a.TranscriptRequired() {
			return "n/a"
		}
		return yes(a.Transcript != nil)
	}
	ts := func(t *time.Time) string {
		if t == nil {
			return ""
		}
		return t.In(eastern).Format("2006-01-02 15:04")
	}
	for _, a := range apps {
		f := a.Form
		var recs []string
		for _, rec := range a.Recommenders {
			sup := ""
			if rec.Supervisor {
				sup = ", supervisor"
			}
			recs = append(recs, rec.Name+" <"+rec.Email+"> ("+rec.Title+sup+"): "+rec.Status)
		}
		cw.Write([]string{
			f.LastName, f.FirstName, a.Email, f.Phone, f.Address1, f.Address2, f.City, f.State, f.Zip,
			f.Program, f.Status, f.StatusNote, f.StartDate, f.GradDate, f.Enrollment,
			f.CreditHours,
			yes(a.Certification != nil), yes(a.Statement != nil), yes(a.Resume != nil), yes(a.FinancialAid != nil),
			transcript(a),
			strconv.Itoa(a.LettersReceived()), yes(a.SupervisorLetterReceived()),
			strings.Join(recs, "; "), yes(a.Complete()), ts(a.CompletedAt), ts(&a.CreatedAt), ts(&a.UpdatedAt),
		})
	}
	cw.Flush()
}

const reminderWindow = 7 * 24 * time.Hour

// handleReminders is called daily by Cloud Scheduler. Within a week of the
// deadline, it sends (once) a reminder to each incomplete applicant and to
// each recommender whose letter is still outstanding.
func (s *server) handleReminders(w http.ResponseWriter, r *http.Request) {
	got := r.Header.Get("X-Cron-Secret")
	if s.cfg.CronSecret == "" || subtle.ConstantTimeCompare([]byte(got), []byte(s.cfg.CronSecret)) != 1 {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	now := s.now()
	result := map[string]int{"applicants": 0, "recommenders": 0, "errors": 0}
	defer func() {
		slog.Info("reminders run", "result", result)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}()
	if now.Before(s.cfg.Deadline.Add(-reminderWindow)) || !now.Before(s.cfg.LetterDeadline) {
		return
	}

	apps, err := s.store.ListApplicants(r.Context())
	if err != nil {
		slog.Error("listing applicants", "err", err)
		result["errors"]++
	}
	for _, a := range apps {
		var remindApplicant bool
		var remindRecs []*Recommender
		updated, err := s.store.Update(r.Context(), a.Email, func(a *Applicant) error {
			remindApplicant, remindRecs = false, nil
			if a.RemindedAt == nil && !a.Complete() && now.Before(s.cfg.Deadline) {
				a.RemindedAt, remindApplicant = &now, true
			}
			for _, rec := range a.Recommenders {
				// skip anyone asked in the last two days; they just got an email
				if rec.Status == recRequested && rec.RemindedAt == nil && now.Sub(rec.RequestedAt) > 48*time.Hour {
					rec.RemindedAt, rec.LastEmailedAt = &now, now
					remindRecs = append(remindRecs, rec)
				}
			}
			return nil
		})
		if err != nil {
			slog.Error("reminders: updating applicant", "email", a.Email, "err", err)
			result["errors"]++
			continue
		}
		a = updated
		if remindApplicant {
			if s.send(r.Context(), applicantReminderEmail(a, s.cfg.BaseURL, s.cfg.Deadline)) == nil {
				result["applicants"]++
			} else {
				result["errors"]++
			}
		}
		for _, rec := range remindRecs {
			if s.send(r.Context(), recommenderRequestEmail(a, rec, s.recommenderLink(a.Email, rec.ID), s.cfg.LetterDeadline, true)) == nil {
				result["recommenders"]++
			} else {
				result["errors"]++
			}
		}
	}
}
