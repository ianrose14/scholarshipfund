package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// All authentication is done with stateless HMAC-signed tokens, so there is
// nothing to store server-side:
//
//   - login:     emailed magic link, valid for loginTTL
//   - session:   cookie set after the magic link is used, valid for sessionTTL
//   - rec:       a recommender's personal upload link; only valid while that
//                recommender (by ID) still exists in the applicant's record

const (
	kindLogin   = "login"
	kindSession = "session"
	kindRec     = "rec"

	loginTTL   = 30 * time.Minute
	sessionTTL = 30 * 24 * time.Hour
)

var errBadToken = errors.New("invalid or expired link")

type claims struct {
	Kind  string `json:"k"`
	Email string `json:"e"`
	RecID string `json:"r,omitempty"`
	Exp   int64  `json:"x"`
}

type signer struct{ key []byte }

func (s signer) sign(c claims) string {
	payload, _ := json.Marshal(c)
	p := base64.RawURLEncoding.EncodeToString(payload)
	return p + "." + base64.RawURLEncoding.EncodeToString(s.mac(p))
}

func (s signer) verify(tok, kind string, now time.Time) (claims, error) {
	p, sig, ok := strings.Cut(tok, ".")
	if !ok {
		return claims{}, errBadToken
	}
	got, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil || !hmac.Equal(got, s.mac(p)) {
		return claims{}, errBadToken
	}
	payload, err := base64.RawURLEncoding.DecodeString(p)
	if err != nil {
		return claims{}, errBadToken
	}
	var c claims
	if err := json.Unmarshal(payload, &c); err != nil {
		return claims{}, errBadToken
	}
	if c.Kind != kind || now.Unix() > c.Exp {
		return claims{}, errBadToken
	}
	return c, nil
}

func (s signer) mac(p string) []byte {
	h := hmac.New(sha256.New, s.key)
	h.Write([]byte(p))
	return h.Sum(nil)
}
