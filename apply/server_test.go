package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

type captureMailer struct {
	mu   sync.Mutex
	sent []Email
}

func (c *captureMailer) Send(_ context.Context, m Email) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, m)
	return nil
}

// last returns the most recent email to addr whose subject contains subj.
func (c *captureMailer) last(t *testing.T, addr, subj string) Email {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := len(c.sent) - 1; i >= 0; i-- {
		m := c.sent[i]
		if strings.Contains(m.Subject, subj) && strings.Contains(strings.Join(m.To, ","), addr) {
			return m
		}
	}
	t.Fatalf("no email to %s with subject containing %q; sent: %+v", addr, subj, c.sent)
	return Email{}
}

func (c *captureMailer) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.sent)
}

type testEnv struct {
	t       *testing.T
	srv     *server
	ts      *httptest.Server
	mail    *captureMailer
	dataDir string
	now     time.Time
}

func newTestEnv(t *testing.T) *testEnv {
	e := &testEnv{t: t, mail: &captureMailer{}, dataDir: t.TempDir(),
		now: time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)}
	cfg := Config{
		SecretKey:      []byte("0123456789abcdef0123456789abcdef"),
		Cycle:          "2027",
		Deadline:       time.Date(2027, 3, 1, 23, 59, 0, 0, eastern),
		LetterDeadline: time.Date(2027, 3, 8, 23, 59, 0, 0, eastern),
		AdminEmails:    map[string]bool{"admin@example.org": true},
		NotifyEmails:   []string{"admin@example.org"},
		StorageRoot:    "/2027",
		CronSecret:     "cron-secret",
	}
	e.srv = newServer(cfg, &Store{fs: &localStorage{base: e.dataDir}, root: cfg.StorageRoot}, e.mail)
	e.srv.now = func() time.Time { return e.now }
	e.ts = httptest.NewServer(e.srv.routes())
	e.srv.cfg.BaseURL = e.ts.URL
	t.Cleanup(e.ts.Close)
	return e
}

// client returns a browser-like client with its own cookie jar that doesn't follow redirects.
func (e *testEnv) client() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func (e *testEnv) do(c *http.Client, req *http.Request) (int, string, http.Header) {
	e.t.Helper()
	resp, err := c.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b), resp.Header
}

func (e *testEnv) get(c *http.Client, path string) (int, string, http.Header) {
	req, _ := http.NewRequest("GET", e.abs(path), nil)
	return e.do(c, req)
}

func (e *testEnv) post(c *http.Client, path string, form url.Values) (int, string, http.Header) {
	req, _ := http.NewRequest("POST", e.abs(path), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return e.do(c, req)
}

func (e *testEnv) upload(c *http.Client, path, filename string, data []byte) (int, string, http.Header) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", filename)
	fw.Write(data)
	mw.Close()
	req, _ := http.NewRequest("POST", e.abs(path), &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return e.do(c, req)
}

func (e *testEnv) abs(p string) string {
	if strings.HasPrefix(p, "http") {
		return p
	}
	return e.ts.URL + p
}

var linkRE = regexp.MustCompile(`http://\S+`)

func (e *testEnv) linkIn(m Email) string {
	e.t.Helper()
	l := linkRE.FindString(m.Text)
	if l == "" {
		e.t.Fatalf("no link in email: %s", m.Text)
	}
	return l
}

// signIn performs the full magic-link flow and returns a signed-in client.
func (e *testEnv) signIn(email string) *http.Client {
	e.t.Helper()
	c := e.client()
	if code, _, _ := e.post(c, "/login", url.Values{"email": {email}}); code != 200 {
		e.t.Fatalf("login: %d", code)
	}
	link := e.linkIn(e.mail.last(e.t, normalizeEmail(email), "sign-in link"))
	code, body, _ := e.get(c, link)
	if code != 200 || !strings.Contains(body, "Continue to my application") {
		e.t.Fatalf("auth confirm page: %d %s", code, body)
	}
	u, _ := url.Parse(link)
	code, _, h := e.post(c, "/auth", url.Values{"t": {u.Query().Get("t")}})
	if code != http.StatusSeeOther {
		e.t.Fatalf("auth: %d", code)
	}
	if h.Get("Location") == "" {
		e.t.Fatal("auth: no redirect")
	}
	return c
}

func (e *testEnv) load(email string) *Applicant {
	e.t.Helper()
	a, _, err := e.srv.store.Load(context.Background(), email)
	if err != nil {
		e.t.Fatal(err)
	}
	return a
}

func expect(t *testing.T, code, want int, body string) {
	t.Helper()
	if code != want {
		t.Fatalf("got status %d, want %d; body:\n%s", code, want, body)
	}
}

func contains(t *testing.T, body string, subs ...string) {
	t.Helper()
	for _, s := range subs {
		if !strings.Contains(body, s) {
			t.Fatalf("body does not contain %q:\n%s", s, body)
		}
	}
}

var (
	fakePDF  = []byte("%PDF-1.4\n% fake pdf for tests\n")
	fakeDOCX = []byte("PK\x03\x04 fake docx")
)

func TestFullApplicationFlow(t *testing.T) {
	e := newTestEnv(t)
	const me = "jane.doe@example.com"

	// unauthenticated users are sent to sign in
	code, _, h := e.get(e.client(), "/app")
	expect(t, code, http.StatusSeeOther, "")
	if h.Get("Location") != "/" {
		t.Fatalf("redirect to %q", h.Get("Location"))
	}

	c := e.signIn("  Jane.Doe@Example.com ") // normalized
	code, body, _ := e.get(c, "/app")
	expect(t, code, 200, body)
	contains(t, body, "Your 2027 application", "Not started", "0 of 2 received")

	// can't add recommenders before giving a name
	code, body, _ = e.post(c, "/app/recommenders", url.Values{"name": {"X"}, "email": {"x@example.com"}, "title": {"Y"}})
	expect(t, code, 400, body)
	contains(t, body, "fill in your name")

	// partial form saves as a draft
	form := url.Values{"first_name": {"Jane"}, "last_name": {"Doé"}}
	code, body, _ = e.post(c, "/app/form", form)
	expect(t, code, 200, body)
	contains(t, body, "Your answers have been saved")
	if a := e.load(me); a.Form.LastName != "Doé" || a.Certification != nil {
		t.Fatalf("draft not saved correctly: %+v", a)
	}

	// signing an incomplete form is refused (but the answers are kept)
	form.Set("certify", "1")
	form.Set("sign_name", "Jane Doé")
	code, body, _ = e.post(c, "/app/form", form)
	expect(t, code, 400, body)
	contains(t, body, "please complete these fields before signing", "Address")

	// complete, signed form
	for k, v := range map[string]string{
		"address1": "1 Main St", "city": "Atlanta", "state": "GA", "zip": "30303", "phone": "404-555-1212",
		"program": "Emory NNP", "status": "applying_parttime", "start_date": "Aug 2027", "enrollment": "parttime",
		"parttime_year1": "6 credits/semester",
	} {
		form.Set(k, v)
	}
	code, body, h = e.post(c, "/app/form", form)
	expect(t, code, http.StatusSeeOther, body)
	a := e.load(me)
	if !a.FormComplete() || a.Certification.Name != "Jane Doé" {
		t.Fatalf("form not complete: %+v", a)
	}
	pdf, err := os.ReadFile(filepath.Join(e.dataDir, "2027", me, appFormFilename))
	if err != nil || !bytes.HasPrefix(pdf, []byte("%PDF")) {
		t.Fatalf("application PDF not written: %v", err)
	}

	// uploads: bad type, then good ones; replacing with another extension removes the old file
	code, body, _ = e.upload(c, "/app/upload/statement", "statement.pdf", []byte("not really a pdf"))
	expect(t, code, 400, body)
	contains(t, body, "PDF or Word document")
	code, body, _ = e.upload(c, "/app/upload/statement", "My Statement.docx", fakeDOCX)
	expect(t, code, http.StatusSeeOther, body)
	code, body, _ = e.upload(c, "/app/upload/statement", "My Statement.pdf", fakePDF)
	expect(t, code, http.StatusSeeOther, body)
	if _, err := os.Stat(filepath.Join(e.dataDir, "2027", me, "Personal Statement.docx")); !os.IsNotExist(err) {
		t.Fatal("replaced .docx statement was not deleted")
	}
	code, body, _ = e.upload(c, "/app/upload/resume", "cv.pdf", fakePDF)
	expect(t, code, http.StatusSeeOther, body)
	code, body, _ = e.upload(c, "/app/upload/financialaid", "faa.pdf", fakePDF)
	expect(t, code, http.StatusSeeOther, body)
	code, _, _ = e.upload(c, "/app/upload/bogus", "x.pdf", fakePDF)
	expect(t, code, 404, "")

	// recommenders
	addRec := func(name, email, supervisor string) string {
		t.Helper()
		v := url.Values{"name": {name}, "email": {email}, "title": {"Charge Nurse"}}
		if supervisor != "" {
			v.Set("supervisor", "1")
		}
		code, body, _ := e.post(c, "/app/recommenders", v)
		expect(t, code, http.StatusSeeOther, body)
		m := e.mail.last(t, email, "Letter of recommendation request")
		contains(t, m.Text, "Jane Doé has listed you")
		return e.linkIn(m)
	}
	link1 := addRec("Rita Recommender", "rita@example.com", "")
	link2 := addRec("Sam Supervisor", "sam@example.com", "yes")
	linkOld := addRec("Pat Pending", "pat@example.com", "")

	code, body, _ = e.post(c, "/app/recommenders", url.Values{"name": {"Dup"}, "email": {"RITA@example.com"}, "title": {"x"}})
	expect(t, code, 400, body)
	contains(t, body, "already added")

	// recommender page shows the applicant's name but none of their other details
	rc := e.client()
	code, body, _ = e.get(rc, link1)
	expect(t, code, 200, body)
	contains(t, body, "Letter of recommendation for Jane Doé", "Dear Rita Recommender")
	for _, secret := range []string{"1 Main St", "404-555-1212", me} {
		if strings.Contains(body, secret) {
			t.Fatalf("recommender page leaks %q", secret)
		}
	}

	// first letter: applicant is notified but application isn't complete yet
	code, body, _ = e.upload(rc, link1+"/upload", "letter.pdf", fakePDF)
	expect(t, code, http.StatusSeeOther, body)
	e.mail.last(t, me, "Letter of recommendation received from Rita Recommender")
	if e.load(me).Complete() {
		t.Fatal("complete with only one letter")
	}

	// once received, a recommender can't be edited or removed by the applicant
	recs := e.load(me).Recommenders
	code, body, _ = e.post(c, "/app/recommenders/"+recs[0].ID+"/delete", nil)
	expect(t, code, 400, body)
	contains(t, body, "can&#39;t be removed")

	// changing a pending recommender's email invalidates the old link
	code, body, _ = e.post(c, "/app/recommenders/"+recs[2].ID, url.Values{"name": {"Pat Pending"}, "email": {"pat.new@example.com"}, "title": {"RN"}})
	expect(t, code, http.StatusSeeOther, body)
	code, _, _ = e.get(e.client(), linkOld)
	expect(t, code, 404, "")
	e.mail.last(t, "pat.new@example.com", "Letter of recommendation request")

	// resend is throttled
	code, body, _ = e.post(c, "/app/recommenders/"+recs[1].ID+"/resend", nil)
	expect(t, code, 400, body)
	e.now = e.now.Add(2 * time.Hour)
	code, body, _ = e.post(c, "/app/recommenders/"+recs[1].ID+"/resend", nil)
	expect(t, code, http.StatusSeeOther, body)

	// supervisor's letter completes the application
	code, body, _ = e.upload(e.client(), link2+"/upload", "sam.docx", fakeDOCX)
	expect(t, code, http.StatusSeeOther, body)
	a = e.load(me)
	if !a.Complete() || a.CompletedAt == nil {
		t.Fatalf("application should be complete: %v", a.MissingItems())
	}
	e.mail.last(t, me, "application is complete")
	e.mail.last(t, "admin@example.org", "Application complete: Jane Doé")
	for _, f := range []string{"Letter - Rita Recommender.pdf", "Letter - Sam Supervisor.docx", "Resume.pdf", "Personal Statement.pdf", "Financial Aid Certification.pdf"} {
		if _, err := os.Stat(filepath.Join(e.dataDir, "2027", me, f)); err != nil {
			t.Errorf("missing file %s: %v", f, err)
		}
	}
	code, body, _ = e.get(c, "/app")
	contains(t, body, "Your application is complete")

	// a third recommender declines
	pat := e.linkIn(e.mail.last(t, "pat.new@example.com", "Letter of recommendation request"))
	code, body, _ = e.post(e.client(), pat+"/decline", nil)
	expect(t, code, http.StatusSeeOther, body)
	e.mail.last(t, me, "Pat Pending is unable to write")

	// admin
	admin := e.signIn("admin@example.org")
	code, body, _ = e.get(admin, "/admin")
	expect(t, code, 200, body)
	contains(t, body, "Jane Doé", "Complete")
	code, body, _ = e.get(admin, "/admin/applicants.csv")
	expect(t, code, 200, body)
	contains(t, body, "Doé,Jane,jane.doe@example.com")
	code, _, _ = e.get(c, "/admin") // applicants can't see it
	expect(t, code, http.StatusSeeOther, "")

	// after the deadline: read-only
	e.now = time.Date(2027, 3, 2, 12, 0, 0, 0, eastern)
	c = e.signIn(me) // the old session has expired by now; existing applicants can still sign in
	code, _, h = e.post(c, "/app/form", form)
	expect(t, code, http.StatusSeeOther, "")
	contains(t, h.Get("Location"), "msg=closed")
	code, body, _ = e.get(c, "/app")
	contains(t, body, "View form")
	if strings.Contains(body, `type="file"`) || strings.Contains(body, "Add a recommender") {
		t.Fatal("dashboard still editable after the deadline")
	}
	// ...but recommenders have until the letter deadline
	code, body, _ = e.get(e.client(), link1)
	contains(t, body, "Replace your letter")
}

func TestLoginEdgeCases(t *testing.T) {
	e := newTestEnv(t)
	c := e.client()

	code, body, _ := e.post(c, "/login", url.Values{"email": {"not-an-email"}})
	expect(t, code, 400, body)

	// per-email rate limit: same response, but no more emails
	for i := 0; i < 5; i++ {
		code, body, _ = e.post(c, "/login", url.Values{"email": {"a@example.com"}})
		expect(t, code, 200, body)
		contains(t, body, "Check your email")
	}
	if n := e.mail.count(); n != 3 {
		t.Fatalf("sent %d login emails, want 3", n)
	}

	// expired link
	link := e.linkIn(e.mail.last(t, "a@example.com", "sign-in"))
	e.now = e.now.Add(loginTTL + time.Minute)
	code, body, _ = e.get(c, link)
	expect(t, code, 400, body)
	contains(t, body, "invalid or has expired")

	// new applicants can't start after the deadline
	e.now = time.Date(2027, 3, 5, 0, 0, 0, 0, time.UTC)
	e.post(c, "/login", url.Values{"email": {"late@example.com"}})
	u, _ := url.Parse(e.linkIn(e.mail.last(t, "late@example.com", "sign-in")))
	code, body, _ = e.post(c, "/auth", url.Values{"t": {u.Query().Get("t")}})
	expect(t, code, 200, body)
	contains(t, body, "Applications are closed")
}

func TestCrossOriginPostRejected(t *testing.T) {
	e := newTestEnv(t)
	req, _ := http.NewRequest("POST", e.abs("/app/form"), strings.NewReader("first_name=x"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	code, _, _ := e.do(e.client(), req)
	expect(t, code, http.StatusForbidden, "")
}

func TestReminders(t *testing.T) {
	e := newTestEnv(t)
	c := e.signIn("jane@example.com")
	e.post(c, "/app/form", url.Values{"first_name": {"Jane"}, "last_name": {"Doe"}})
	e.post(c, "/app/recommenders", url.Values{"name": {"Rita"}, "email": {"rita@example.com"}, "title": {"RN"}})

	run := func(secret string) (int, map[string]int) {
		req, _ := http.NewRequest("POST", e.abs("/tasks/reminders"), nil)
		req.Header.Set("X-Cron-Secret", secret)
		code, body, _ := e.do(e.client(), req)
		var res map[string]int
		json.Unmarshal([]byte(body), &res)
		return code, res
	}
	if code, _ := run("wrong"); code != 403 {
		t.Fatalf("bad secret: %d", code)
	}
	// too early: nothing
	if _, res := run("cron-secret"); res["applicants"] != 0 || res["recommenders"] != 0 {
		t.Fatalf("early run sent reminders: %v", res)
	}
	// within a week of the deadline: one of each, exactly once
	e.now = time.Date(2027, 2, 25, 12, 0, 0, 0, time.UTC)
	if _, res := run("cron-secret"); res["applicants"] != 1 || res["recommenders"] != 1 {
		t.Fatalf("reminders: %v", res)
	}
	contains(t, e.mail.last(t, "jane@example.com", "Reminder").Text, "Personal statement", "Letters of recommendation (0 of 2 received)")
	contains(t, e.mail.last(t, "rita@example.com", "Reminder: Letter of recommendation").Text, "friendly reminder")
	if _, res := run("cron-secret"); res["applicants"] != 0 || res["recommenders"] != 0 {
		t.Fatalf("second run re-sent reminders: %v", res)
	}
}

func TestTokens(t *testing.T) {
	s := signer{key: []byte("k")}
	now := time.Unix(1000, 0)
	tok := s.sign(claims{Kind: kindLogin, Email: "a@b.c", Exp: 2000})
	if c, err := s.verify(tok, kindLogin, now); err != nil || c.Email != "a@b.c" {
		t.Fatalf("valid token rejected: %v", err)
	}
	if _, err := s.verify(tok, kindSession, now); err == nil {
		t.Fatal("token accepted as wrong kind")
	}
	if _, err := s.verify(tok, kindLogin, time.Unix(2001, 0)); err == nil {
		t.Fatal("expired token accepted")
	}
	if _, err := (signer{key: []byte("other")}).verify(tok, kindLogin, now); err == nil {
		t.Fatal("token with wrong key accepted")
	}
	p, sig, _ := strings.Cut(tok, ".")
	if _, err := s.verify(p+"x."+sig, kindLogin, now); err == nil {
		t.Fatal("tampered token accepted")
	}
}

func TestDropboxArgEscapesNonASCII(t *testing.T) {
	got := dropboxArg(map[string]string{"path": "/2027/a/Letter - José 😀.pdf"})
	if want := `{"path":"/2027/a/Letter - Jos\u00e9 \ud83d\ude00.pdf"}`; got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestFillApplicationPDF(t *testing.T) {
	a := &Applicant{Email: "a@example.com", Form: ApplicationForm{FirstName: "Jane", LastName: "Doe", Status: "other",
		StatusNote: strings.Repeat("word ", 50), Enrollment: "fulltime"},
		Certification: &Certification{Name: "Jane Doe", At: time.Now()}}
	pdf, err := fillApplicationPDF(a)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF")) || len(pdf) < 1000 {
		t.Fatal("output doesn't look like a PDF")
	}
}
