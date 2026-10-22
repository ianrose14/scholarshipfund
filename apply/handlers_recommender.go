package main

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
)

// recommenderFromToken resolves a recommender's personal link. It writes an
// error page and returns ok=false if the link is invalid or the recommender
// has since been removed (or had their email changed).
func (s *server) recommenderFromToken(w http.ResponseWriter, r *http.Request) (a *Applicant, rec *Recommender, ok bool) {
	c, err := s.tokens.verify(r.PathValue("token"), kindRec, s.now())
	if err == nil {
		a, _, err = s.store.Load(r.Context(), c.Email)
	}
	if err == nil {
		if rec = a.recommender(c.RecID); rec == nil {
			err = errNotFound
		}
	}
	switch {
	case err == nil:
		return a, rec, true
	case errors.Is(err, errBadToken), errors.Is(err, errNotFound):
		s.message(w, r, http.StatusNotFound, "Link no longer valid",
			"This link is invalid or no longer active. The applicant may have updated their list of recommenders. If you believe this is a mistake, please reply to the email you received.")
	default:
		s.serverError(w, r, err)
	}
	return nil, nil, false
}

func (s *server) recommenderPage(a *Applicant, rec *Recommender, token, errMsg string) pageData {
	// Only the applicant's name is shown to recommenders; templates must not use other fields.
	return pageData{Title: "Letter of recommendation", A: &Applicant{Form: ApplicationForm{FirstName: a.Form.FirstName, LastName: a.Form.LastName}},
		Rec: rec, Token: token, Error: errMsg, Deadline: s.cfg.LetterDeadline}
}

func (s *server) lettersOpen() bool { return s.now().Before(s.cfg.LetterDeadline) }

func (s *server) handleRecommenderPage(w http.ResponseWriter, r *http.Request) {
	a, rec, ok := s.recommenderFromToken(w, r)
	if !ok {
		return
	}
	s.renderRecommender(w, r, http.StatusOK, s.recommenderPage(a, rec, r.PathValue("token"), ""))
}

func (s *server) renderRecommender(w http.ResponseWriter, r *http.Request, status int, d pageData) {
	d.NoNav = true // recommenders aren't signed in
	s.render(w, r, "recommender", status, d)
}

func (s *server) handleRecommenderUpload(w http.ResponseWriter, r *http.Request) {
	a, rec, ok := s.recommenderFromToken(w, r)
	if !ok {
		return
	}
	token := r.PathValue("token")
	if !s.lettersOpen() {
		s.renderRecommender(w, r, http.StatusBadRequest, s.recommenderPage(a, rec, token, "The deadline for letters has passed. Please reply to the request email if you still wish to submit a letter."))
		return
	}
	data, orig, ext, err := readUpload(w, r, "file")
	if err != nil {
		var ue userError
		if !errors.As(err, &ue) {
			s.serverError(w, r, err)
			return
		}
		s.renderRecommender(w, r, http.StatusBadRequest, s.recommenderPage(a, rec, token, ue.Error()))
		return
	}

	name := letterFilename(a, rec, ext)
	if err := s.store.PutFile(r.Context(), a.Email, name, data); err != nil {
		s.serverError(w, r, err)
		return
	}
	var old string
	var firstLetter bool
	a, err = s.update(r.Context(), a.Email, func(a *Applicant) error {
		cur := a.recommender(rec.ID)
		if cur == nil {
			return errNotFound // removed while uploading; very unlikely
		}
		now := s.now()
		old, firstLetter = "", cur.Status != recReceived
		if cur.Letter != nil {
			old = cur.Letter.Filename
		}
		cur.Status, cur.RespondedAt = recReceived, &now
		cur.Letter = &Upload{Filename: name, OriginalName: orig, Size: int64(len(data)), UploadedAt: now}
		rec = cur
		return nil
	})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	slog.Info("letter received", "applicant", a.Email, "recommender", rec.Email)
	if old != "" && old != name {
		if err := s.store.DeleteFile(r.Context(), a.Email, old); err != nil {
			slog.Warn("deleting replaced letter", "email", a.Email, "file", old, "err", err)
		}
	}
	if firstLetter {
		s.send(r.Context(), letterReceivedEmail(a, rec, s.cfg.BaseURL+"/app"))
	}
	http.Redirect(w, r, "/r/"+url.PathEscape(token)+"?msg=letter-thanks", http.StatusSeeOther)
}

// letterFilename picks a name like "Letter - Jane Smith.pdf" that doesn't
// collide with another recommender's letter.
func letterFilename(a *Applicant, rec *Recommender, ext string) string {
	base := "Letter - " + safeFilename(rec.Name)
	for n := 1; ; n++ {
		name := base + ext
		if n > 1 {
			name = base + " (" + strconv.Itoa(n) + ")" + ext
		}
		taken := false
		for _, other := range a.Recommenders {
			if other.ID != rec.ID && other.Letter != nil && other.Letter.Filename == name {
				taken = true
			}
		}
		if !taken {
			return name
		}
	}
}

func (s *server) handleRecommenderDecline(w http.ResponseWriter, r *http.Request) {
	a, rec, ok := s.recommenderFromToken(w, r)
	if !ok {
		return
	}
	token := r.PathValue("token")
	if rec.Status == recReceived {
		s.renderRecommender(w, r, http.StatusBadRequest, s.recommenderPage(a, rec, token, "We've already received your letter."))
		return
	}
	var changed bool
	a, err := s.update(r.Context(), a.Email, func(a *Applicant) error {
		cur := a.recommender(rec.ID)
		if cur == nil {
			return errNotFound
		}
		changed = cur.Status == recRequested
		if changed {
			now := s.now()
			cur.Status, cur.RespondedAt = recDeclined, &now
		}
		rec = cur
		return nil
	})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if changed {
		s.send(r.Context(), recommenderDeclinedEmail(a, rec, s.cfg.BaseURL+"/app"))
	}
	http.Redirect(w, r, "/r/"+url.PathEscape(token)+"?msg=declined", http.StatusSeeOther)
}
