package main

import (
	"fmt"
	"strings"
	"time"
)

const fundName = "Dr. Allison Rose Memorial Fund"

var eastern = mustLoadLocation("America/New_York")

func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

func formatDeadline(t time.Time) string {
	return t.In(eastern).Format("Monday, January 2, 2006 at 3:04 PM MST")
}

func emailFooter() string {
	return "\n\n--\n" + fundName + "\nhttps://allisonrosememorialfund.org\nQuestions? Just reply to this email."
}

func loginEmail(to, link string) Email {
	return Email{
		To:      []string{to},
		Subject: "Your sign-in link for the " + fundName + " application",
		Text: fmt.Sprintf(`Hello,

Use the link below to sign in to your scholarship application. It expires in %d minutes.

%s

If you didn't request this, you can safely ignore this email.`, int(loginTTL.Minutes()), link) + emailFooter(),
	}
}

func recommenderRequestEmail(a *Applicant, r *Recommender, link string, deadline time.Time, reminder bool) Email {
	subject := fmt.Sprintf("Letter of recommendation request for %s", a.DisplayName())
	intro := fmt.Sprintf("%s has listed you as a recommender for their application to the %s scholarship", a.DisplayName(), fundName)
	if reminder {
		subject = "Reminder: " + subject
		intro = fmt.Sprintf("This is a friendly reminder that %s has listed you as a recommender for their application to the %s scholarship", a.DisplayName(), fundName)
	}
	return Email{
		To:      []string{r.Email},
		Subject: subject,
		Text: fmt.Sprintf(`Dear %s,

%s. The fund provides tuition assistance to nurses pursuing a Neonatal Nurse Practitioner (NNP) degree.

Please upload your letter (PDF or Word document) using your personal link:

%s

Letters are due by %s.

Your letter is confidential: %s will be told that it was received, but will not be able to see it.

If you are unable to write a letter, you can let them know using the same link.

Thank you for taking the time to support their application!

Sincerely,

The Board of Directors`,
			r.Name, intro, link, formatDeadline(deadline), a.DisplayName()) + emailFooter(),
	}
}

func letterReceivedEmail(a *Applicant, r *Recommender, appURL string) Email {
	return Email{
		To:      []string{a.Email},
		Subject: "Letter of recommendation received from " + r.Name,
		Text: fmt.Sprintf(`Hello %s,

We've received a letter of recommendation from %s.

You can check the status of your application at any time:
%s`, a.Form.FirstName, r.Name, appURL) + emailFooter(),
	}
}

func recommenderDeclinedEmail(a *Applicant, r *Recommender, appURL string) Email {
	return Email{
		To:      []string{a.Email},
		Subject: r.Name + " is unable to write a letter of recommendation",
		Text: fmt.Sprintf(`Hello %s,

%s has let us know that they are unable to write a letter of recommendation for you.

You may want to add a different recommender here:
%s`, a.Form.FirstName, r.Name, appURL) + emailFooter(),
	}
}

func applicationCompleteEmail(a *Applicant) Email {
	return Email{
		To:      []string{a.Email},
		Subject: "Your " + fundName + " application is complete",
		Text: fmt.Sprintf(`Hello %s,

Thank you! We have received all of your application materials. Qualified candidates will be contacted by email to schedule an interview.`, a.Form.FirstName) + emailFooter(),
	}
}

func adminCompleteEmail(to []string, a *Applicant, adminURL string) Email {
	return Email{
		To:      to,
		Subject: "Application complete: " + a.DisplayName(),
		Text:    fmt.Sprintf("%s (%s) has completed their application.\n\n%s\n", a.DisplayName(), a.Email, adminURL),
	}
}

func applicantReminderEmail(a *Applicant, appURL string, deadline time.Time) Email {
	return Email{
		To:      []string{a.Email},
		Subject: "Reminder: your " + fundName + " application is due " + deadline.In(eastern).Format("January 2"),
		Text: fmt.Sprintf(`Hello %s,

Your scholarship application is due by %s. We are still waiting on:

  - %s

You can finish your application here:
%s`, a.Form.FirstName, formatDeadline(deadline), strings.Join(a.MissingItems(), "\n  - "), appURL) + emailFooter(),
	}
}
