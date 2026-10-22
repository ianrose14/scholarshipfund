package main

import (
	"fmt"
	"strings"
	"time"
)

// Applicant is stored as applicant.json in the applicant's Dropbox folder,
// alongside their uploaded files.
type Applicant struct {
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Form          ApplicationForm `json:"application_form"`
	Certification *Certification  `json:"certification,omitempty"`

	Statement    *Upload `json:"personal_statement,omitempty"`
	Resume       *Upload `json:"resume,omitempty"`
	FinancialAid *Upload `json:"financial_aid_certification,omitempty"`

	Recommenders []*Recommender `json:"recommenders"`

	CompletedAt *time.Time `json:"completed_at,omitempty"`
	RemindedAt  *time.Time `json:"reminded_at,omitempty"`
}

// ApplicationForm mirrors the fields of forms/application_form_v1.pdf.
type ApplicationForm struct {
	FirstName  string `json:"first_name"`
	LastName   string `json:"last_name"`
	Address1   string `json:"address1"`
	Address2   string `json:"address2"`
	City       string `json:"city"`
	State      string `json:"state"`
	Zip        string `json:"zip"`
	Phone      string `json:"phone"`
	Program    string `json:"program"`
	Status     string `json:"status"` // one of programStatuses
	StatusNote string `json:"status_other"`
	StartDate  string `json:"start_date"`
	Enrollment string `json:"enrollment"` // "fulltime" or "parttime"

	PartTimeYear1 string `json:"parttime_year1"`
	PartTimeYear2 string `json:"parttime_year2"`
	PartTimeYear3 string `json:"parttime_year3"`
}

type option struct{ Value, Label string }

var programStatuses = []option{
	{"enrolled_fulltime", "I am already enrolled in the FULL-time NNP program listed above."},
	{"enrolled_parttime", "I am already enrolled in the PART-time NNP program listed above."},
	{"applying_fulltime", "I am currently applying to the FULL-time NNP program(s) listed above."},
	{"applying_parttime", "I am currently applying to the PART-time NNP program(s) listed above."},
	{"other", "Other (please explain)"},
}

var enrollmentOptions = []option{{"fulltime", "Full-time"}, {"parttime", "Part-time"}}

func validOption(opts []option, v string) bool {
	for _, o := range opts {
		if o.Value == v {
			return true
		}
	}
	return false
}

type Certification struct {
	Name string    `json:"name"`
	At   time.Time `json:"at"`
	IP   string    `json:"ip"`
}

type Upload struct {
	Filename     string    `json:"filename"` // name within the applicant's folder
	OriginalName string    `json:"original_name"`
	Size         int64     `json:"size"`
	UploadedAt   time.Time `json:"uploaded_at"`
}

const (
	recRequested = "requested"
	recReceived  = "received"
	recDeclined  = "declined"
)

type Recommender struct {
	ID         string `json:"id"` // changes whenever the email changes, invalidating old links
	Name       string `json:"name"`
	Email      string `json:"email"`
	Title      string `json:"title"`
	Supervisor bool   `json:"supervisor"` // current or recent (last 12 months) supervisor

	Status        string     `json:"status"`
	RequestedAt   time.Time  `json:"requested_at"`
	LastEmailedAt time.Time  `json:"last_emailed_at"`
	RemindedAt    *time.Time `json:"reminded_at,omitempty"`
	RespondedAt   *time.Time `json:"responded_at,omitempty"`
	Letter        *Upload    `json:"letter,omitempty"`
}

const (
	lettersRequired   = 2
	maxRecommenders   = 5
	appFormFilename   = "Application Form.pdf"
	applicantJSONFile = "applicant.json"
)

func (a *Applicant) DisplayName() string {
	n := strings.TrimSpace(a.Form.FirstName + " " + a.Form.LastName)
	if n == "" {
		return a.Email
	}
	return n
}

// FormMissing returns labels of required application-form fields that are empty.
func (a *Applicant) FormMissing() []string {
	f := a.Form
	var missing []string
	req := func(v, label string) {
		if strings.TrimSpace(v) == "" {
			missing = append(missing, label)
		}
	}
	req(f.FirstName, "First name")
	req(f.LastName, "Last name")
	req(f.Address1, "Address")
	req(f.City, "City")
	req(f.State, "State")
	req(f.Zip, "Zip code")
	req(f.Phone, "Phone")
	req(f.Program, "NNP program")
	req(f.Status, "Program status")
	if f.Status == "other" {
		req(f.StatusNote, "Program status explanation")
	}
	req(f.StartDate, "Intended start date")
	req(f.Enrollment, "Planned enrollment status")
	if f.Enrollment == "parttime" {
		req(f.PartTimeYear1, "Part-time course of study")
	}
	return missing
}

func (a *Applicant) FormComplete() bool {
	return len(a.FormMissing()) == 0 && a.Certification != nil
}

func (a *Applicant) LettersReceived() int {
	n := 0
	for _, r := range a.Recommenders {
		if r.Status == recReceived {
			n++
		}
	}
	return n
}

func (a *Applicant) SupervisorLetterReceived() bool {
	for _, r := range a.Recommenders {
		if r.Status == recReceived && r.Supervisor {
			return true
		}
	}
	return false
}

func (a *Applicant) LettersComplete() bool {
	return a.LettersReceived() >= lettersRequired && a.SupervisorLetterReceived()
}

func (a *Applicant) Complete() bool {
	return a.FormComplete() && a.Statement != nil && a.Resume != nil && a.FinancialAid != nil && a.LettersComplete()
}

// checklistItems is the number of items on the applicant's dashboard checklist.
const checklistItems = 5

// ItemsComplete counts how many of the checklistItems are done.
func (a *Applicant) ItemsComplete() int {
	n := 0
	for _, done := range []bool{a.FormComplete(), a.Statement != nil, a.Resume != nil, a.FinancialAid != nil, a.LettersComplete()} {
		if done {
			n++
		}
	}
	return n
}

func (a *Applicant) ItemsTotal() int { return checklistItems }

// MissingItems is a human-readable list of what's outstanding, used in emails
// and the admin page.
func (a *Applicant) MissingItems() []string {
	var out []string
	if !a.FormComplete() {
		out = append(out, "Application form")
	}
	if a.Statement == nil {
		out = append(out, "Personal statement")
	}
	if a.Resume == nil {
		out = append(out, "Resume / CV")
	}
	if a.FinancialAid == nil {
		out = append(out, "Financial aid certification")
	}
	if n := a.LettersReceived(); n < lettersRequired {
		out = append(out, fmt.Sprintf("Letters of recommendation (%d of %d received)", n, lettersRequired))
	} else if !a.SupervisorLetterReceived() {
		out = append(out, "A letter from a current or recent supervisor")
	}
	return out
}

func (a *Applicant) recommender(id string) *Recommender {
	for _, r := range a.Recommenders {
		if r.ID == id {
			return r
		}
	}
	return nil
}
