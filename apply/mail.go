package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

type Email struct {
	To      []string
	Subject string
	Text    string
}

type Mailer interface {
	Send(ctx context.Context, m Email) error
}

// resendMailer sends via https://resend.com/docs/api-reference/emails/send-email
type resendMailer struct {
	apiKey, from, replyTo string
	client                *http.Client
}

func (r *resendMailer) Send(ctx context.Context, m Email) error {
	body, _ := json.Marshal(map[string]any{
		"from":     r.from,
		"to":       m.To,
		"reply_to": r.replyTo,
		"subject":  m.Subject,
		"text":     m.Text,
	})
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.resend.com/emails", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("resend: %s: %s", resp.Status, b)
	}
	slog.Info("email sent", "to", m.To, "subject", m.Subject)
	return nil
}

func newResendMailer(apiKey, from, replyTo string) *resendMailer {
	return &resendMailer{apiKey: apiKey, from: from, replyTo: replyTo, client: &http.Client{Timeout: 30 * time.Second}}
}

// logMailer prints emails instead of sending them (development).
type logMailer struct{}

func (logMailer) Send(_ context.Context, m Email) error {
	fmt.Printf("\n===== EMAIL to %v =====\nSubject: %s\n\n%s\n======================\n\n", m.To, m.Subject, m.Text)
	return nil
}
