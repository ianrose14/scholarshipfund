package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

func (s *server) handleDashboard(w http.ResponseWriter, r *http.Request, a *Applicant) {
	s.render(w, r, "dashboard", http.StatusOK, pageData{Title: "My application", A: a})
}

// dashboardError re-renders the dashboard with a message for a userError, or
// fails the request for anything else.
func (s *server) dashboardError(w http.ResponseWriter, r *http.Request, a *Applicant, err error) {
	var ue userError
	if !errors.As(err, &ue) {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, "dashboard", http.StatusBadRequest, pageData{Title: "My application", A: a, Error: ue.Error()})
}

// ---- application form ----

func (s *server) formPage(a *Applicant, f ApplicationForm, errMsg string) pageData {
	d := pageData{Title: "Application form", A: a, Form: f, Error: errMsg,
		Statuses: programStatuses, Enrollments: enrollmentOptions, Certify: a.Certification != nil}
	if a.Certification != nil {
		d.SignName = a.Certification.Name
	}
	return d
}

func (s *server) handleFormPage(w http.ResponseWriter, r *http.Request, a *Applicant) {
	s.render(w, r, "form", http.StatusOK, s.formPage(a, a.Form, ""))
}

// formValue returns a trimmed, length-limited form value.
func formValue(r *http.Request, key string, max int) string {
	v := strings.Join(strings.Fields(r.FormValue(key)), " ")
	if rs := []rune(v); len(rs) > max {
		v = string(rs[:max])
	}
	return v
}

func (s *server) handleFormSave(w http.ResponseWriter, r *http.Request, a *Applicant) {
	f := ApplicationForm{
		FirstName:     formValue(r, "first_name", 100),
		LastName:      formValue(r, "last_name", 100),
		Address1:      formValue(r, "address1", 200),
		Address2:      formValue(r, "address2", 200),
		City:          formValue(r, "city", 100),
		State:         formValue(r, "state", 50),
		Zip:           formValue(r, "zip", 20),
		Phone:         formValue(r, "phone", 40),
		Program:       formValue(r, "program", 200),
		Status:        formValue(r, "status", 40),
		StatusNote:    formValue(r, "status_other", 200),
		StartDate:     formValue(r, "start_date", 40),
		Enrollment:    formValue(r, "enrollment", 20),
		PartTimeYear1: formValue(r, "parttime_year1", 120),
		PartTimeYear2: formValue(r, "parttime_year2", 120),
		PartTimeYear3: formValue(r, "parttime_year3", 120),
	}
	if f.Status != "" && !validOption(programStatuses, f.Status) {
		f.Status = ""
	}
	if f.Enrollment != "" && !validOption(enrollmentOptions, f.Enrollment) {
		f.Enrollment = ""
	}
	certify := r.FormValue("certify") != ""
	signName := formValue(r, "sign_name", 200)

	// Always save what they entered (so nothing is lost), but only record the
	// certification if the form is complete and signed.
	var errMsg string
	draft := Applicant{Form: f}
	switch missing := draft.FormMissing(); {
	case certify && len(missing) > 0:
		errMsg = "Your answers have been saved, but please complete these fields before signing: " + strings.Join(missing, ", ") + "."
		certify = false
	case certify && signName == "":
		errMsg = "Your answers have been saved. To sign, please type your full name."
		certify = false
	}
	now := s.now()
	updated, err := s.update(r.Context(), a.Email, func(a *Applicant) error {
		a.Form = f
		a.Certification = nil
		if certify {
			a.Certification = &Certification{Name: signName, At: now, IP: clientIP(r)}
		}
		return nil
	})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	a = updated

	pdf, err := fillApplicationPDF(a)
	if err == nil {
		err = s.store.PutFile(r.Context(), a.Email, appFormFilename, pdf)
	}
	if err != nil {
		s.serverError(w, r, fmt.Errorf("writing application PDF: %w", err))
		return
	}

	if errMsg != "" {
		d := s.formPage(a, f, errMsg)
		d.SignName = signName
		s.render(w, r, "form", http.StatusBadRequest, d)
		return
	}
	if !certify {
		d := s.formPage(a, f, "")
		d.Flash = "Your answers have been saved. When you're ready, sign the form at the bottom to complete this step."
		s.render(w, r, "form", http.StatusOK, d)
		return
	}
	s.redirectFlash(w, r, "/app", "saved")
}

// ---- file uploads ----

const maxUploadBytes = 20 << 20

var fileSignatures = map[string][]byte{
	".pdf":  []byte("%PDF-"),
	".docx": []byte("PK\x03\x04"),
	".doc":  {0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1},
}

// readUpload reads and validates an uploaded PDF or Word document.
func readUpload(w http.ResponseWriter, r *http.Request, field string) (data []byte, origName, ext string, err error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes+1<<20)
	file, hdr, err := r.FormFile(field)
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return nil, "", "", userError("That file is too large. The maximum size is 20 MB.")
		}
		return nil, "", "", userError("Please choose a file to upload.")
	}
	defer file.Close()
	data, err = io.ReadAll(io.LimitReader(file, maxUploadBytes+1))
	if err != nil {
		return nil, "", "", err
	}
	if len(data) > maxUploadBytes {
		return nil, "", "", userError("That file is too large. The maximum size is 20 MB.")
	}
	ext = strings.ToLower(filepath.Ext(hdr.Filename))
	sig, ok := fileSignatures[ext]
	if !ok || !bytes.HasPrefix(data, sig) {
		return nil, "", "", userError("Please upload a PDF or Word document (.pdf, .doc or .docx).")
	}
	return data, hdr.Filename, ext, nil
}

var uploadKinds = map[string]string{
	"statement":    "Personal Statement",
	"resume":       "Resume",
	"financialaid": "Financial Aid Certification",
}

func (s *server) handleUpload(w http.ResponseWriter, r *http.Request, a *Applicant) {
	kind := r.PathValue("kind")
	base, ok := uploadKinds[kind]
	if !ok {
		http.NotFound(w, r)
		return
	}
	data, orig, ext, err := readUpload(w, r, "file")
	if err != nil {
		s.dashboardError(w, r, a, err)
		return
	}
	name := base + ext
	if err := s.store.PutFile(r.Context(), a.Email, name, data); err != nil {
		s.serverError(w, r, err)
		return
	}
	var old string
	_, err = s.update(r.Context(), a.Email, func(a *Applicant) error {
		up := &Upload{Filename: name, OriginalName: orig, Size: int64(len(data)), UploadedAt: s.now()}
		slot := &a.Statement
		switch kind {
		case "resume":
			slot = &a.Resume
		case "financialaid":
			slot = &a.FinancialAid
		}
		old = ""
		if *slot != nil {
			old = (*slot).Filename
		}
		*slot = up
		return nil
	})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if old != "" && old != name { // e.g. replaced a .docx with a .pdf
		if err := s.store.DeleteFile(r.Context(), a.Email, old); err != nil {
			slog.Warn("deleting replaced file", "email", a.Email, "file", old, "err", err)
		}
	}
	s.redirectFlash(w, r, "/app", "uploaded")
}

// ---- recommenders ----

func (s *server) recommenderLink(applicantEmail, recID string) string {
	tok := s.tokens.sign(claims{Kind: kindRec, Email: applicantEmail, RecID: recID,
		// outlive the deadline so late visitors see "deadline passed" rather than "invalid link"
		Exp: s.cfg.LetterDeadline.Add(60 * 24 * time.Hour).Unix()})
	return s.cfg.BaseURL + "/r/" + url.PathEscape(tok)
}

func (s *server) sendRecommenderRequest(r *http.Request, a *Applicant, rec *Recommender, reminder bool) error {
	return s.send(r.Context(), recommenderRequestEmail(a, rec, s.recommenderLink(a.Email, rec.ID), s.cfg.LetterDeadline, reminder))
}

type recommenderInput struct {
	Name, Email, Title string
	Supervisor         bool
}

func readRecommender(r *http.Request, a *Applicant, exceptID string) (recommenderInput, error) {
	in := recommenderInput{
		Name:       formValue(r, "name", 100),
		Email:      normalizeEmail(formValue(r, "email", 254)),
		Title:      formValue(r, "title", 200),
		Supervisor: r.FormValue("supervisor") != "",
	}
	switch {
	case in.Name == "" || in.Email == "" || in.Title == "":
		return in, userError("Please enter the recommender's name, email address and title/position.")
	case !validEmail(in.Email):
		return in, userError("Please enter a valid email address for " + in.Name + ".")
	case in.Email == a.Email:
		return in, userError("A recommender can't use your own email address.")
	}
	for _, rec := range a.Recommenders {
		if rec.ID != exceptID && rec.Email == in.Email {
			return in, userError("You've already added " + in.Email + " as a recommender.")
		}
	}
	return in, nil
}

func (s *server) handleAddRecommender(w http.ResponseWriter, r *http.Request, a *Applicant) {
	if a.Form.FirstName == "" || a.Form.LastName == "" {
		s.dashboardError(w, r, a, userError("Please fill in your name on the application form first, so your recommenders know who the request is from."))
		return
	}
	var rec *Recommender
	updated, err := s.update(r.Context(), a.Email, func(a *Applicant) error {
		if len(a.Recommenders) >= maxRecommenders {
			return userError(fmt.Sprintf("You can list at most %d recommenders.", maxRecommenders))
		}
		in, err := readRecommender(r, a, "")
		if err != nil {
			return err
		}
		now := s.now()
		rec = &Recommender{ID: newID(), Name: in.Name, Email: in.Email, Title: in.Title, Supervisor: in.Supervisor,
			Status: recRequested, RequestedAt: now, LastEmailedAt: now}
		a.Recommenders = append(a.Recommenders, rec)
		return nil
	})
	if err != nil {
		s.dashboardError(w, r, a, err)
		return
	}
	a = updated
	if err := s.sendRecommenderRequest(r, a, rec, false); err != nil {
		s.dashboardError(w, r, a, userError("We saved "+rec.Name+" as a recommender, but couldn't send them an email. Please double-check the address and use \"Resend request\"."))
		return
	}
	s.redirectFlash(w, r, "/app", "rec-added")
}

func (s *server) handleEditRecommender(w http.ResponseWriter, r *http.Request, a *Applicant) {
	id := r.PathValue("id")
	var rec *Recommender
	var emailChanged bool
	updated, err := s.update(r.Context(), a.Email, func(a *Applicant) error {
		rec = a.recommender(id)
		if rec == nil {
			return userError("That recommender no longer exists.")
		}
		if rec.Status == recReceived {
			return userError("We've already received " + rec.Name + "'s letter, so their details can no longer be changed.")
		}
		in, err := readRecommender(r, a, id)
		if err != nil {
			return err
		}
		emailChanged = in.Email != rec.Email
		rec.Name, rec.Title, rec.Supervisor = in.Name, in.Title, in.Supervisor
		if emailChanged {
			now := s.now()
			// New ID: the link sent to the old address stops working.
			rec.ID, rec.Email, rec.Status = newID(), in.Email, recRequested
			rec.RequestedAt, rec.LastEmailedAt, rec.RemindedAt, rec.RespondedAt = now, now, nil, nil
		}
		return nil
	})
	if err != nil {
		s.dashboardError(w, r, a, err)
		return
	}
	a = updated
	if !emailChanged {
		s.redirectFlash(w, r, "/app", "rec-updated")
		return
	}
	if err := s.sendRecommenderRequest(r, a, rec, false); err != nil {
		s.dashboardError(w, r, a, userError("We updated "+rec.Name+"'s details, but couldn't send them an email. Please double-check the address and use \"Resend request\"."))
		return
	}
	s.redirectFlash(w, r, "/app", "rec-updated-emailed")
}

func (s *server) handleDeleteRecommender(w http.ResponseWriter, r *http.Request, a *Applicant) {
	id := r.PathValue("id")
	updated, err := s.update(r.Context(), a.Email, func(a *Applicant) error {
		for i, rec := range a.Recommenders {
			if rec.ID != id {
				continue
			}
			if rec.Status == recReceived {
				return userError("We've already received " + rec.Name + "'s letter, so they can't be removed. Please contact us if you need to change this.")
			}
			a.Recommenders = append(a.Recommenders[:i], a.Recommenders[i+1:]...)
			return nil
		}
		return nil // already gone
	})
	if err != nil {
		s.dashboardError(w, r, a, err)
		return
	}
	a = updated
	s.redirectFlash(w, r, "/app", "rec-removed")
}

const resendInterval = time.Hour

func (s *server) handleResendRecommender(w http.ResponseWriter, r *http.Request, a *Applicant) {
	id := r.PathValue("id")
	var rec *Recommender
	updated, err := s.update(r.Context(), a.Email, func(a *Applicant) error {
		rec = a.recommender(id)
		switch {
		case rec == nil:
			return userError("That recommender no longer exists.")
		case rec.Status != recRequested:
			return userError("There's no outstanding request for " + rec.Name + ".")
		case s.now().Sub(rec.LastEmailedAt) < resendInterval:
			return userError("We emailed " + rec.Name + " less than an hour ago. Please give them some time (and maybe ask them to check their spam folder).")
		}
		rec.LastEmailedAt = s.now()
		return nil
	})
	if err != nil {
		s.dashboardError(w, r, a, err)
		return
	}
	a = updated
	if err := s.sendRecommenderRequest(r, a, rec, true); err != nil {
		s.dashboardError(w, r, a, userError("Sorry, we couldn't send the email to "+rec.Email+". Please double-check the address."))
		return
	}
	s.redirectFlash(w, r, "/app", "rec-resent")
}
