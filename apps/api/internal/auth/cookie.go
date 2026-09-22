package auth

import (
	"net/http"
	"time"
)

// Cookies issues and clears the session cookie.
//
// The cookie is HttpOnly so that XSS in the SPA cannot read it, SameSite=Lax so
// that a third-party site cannot drive state-changing requests with it, and
// Secure whenever the deployment is served over HTTPS.
type Cookies struct {
	Name   string
	Secure bool
	TTL    time.Duration
}

func (c Cookies) Set(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     c.Name,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   c.Secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(c.TTL),
		MaxAge:   int(c.TTL.Seconds()),
	})
}

func (c Cookies) Clear(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     c.Name,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   c.Secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
}

func (c Cookies) Read(r *http.Request) string {
	cookie, err := r.Cookie(c.Name)
	if err != nil {
		return ""
	}
	return cookie.Value
}
