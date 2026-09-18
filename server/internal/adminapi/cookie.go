package adminapi

import (
	"net/http"
	"time"
)

const sessionCookieName = "mesh_session"
const csrfCookieName = "mesh_csrf"
const csrfHeaderName = "X-CSRF-Token"

func setSessionCookie(w http.ResponseWriter, secure bool, token string, expiresAt time.Time) {
	sameSite := http.SameSiteLaxMode
	if secure {
		sameSite = http.SameSiteNoneMode
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
		Expires:  expiresAt,
	})
}

func clearSessionCookie(w http.ResponseWriter, secure bool) {
	setSessionCookie(w, secure, "", time.Unix(0, 0))
}

func setCSRFCookie(w http.ResponseWriter, secure bool, token string, expiresAt time.Time) {
	sameSite := http.SameSiteLaxMode
	if secure {
		sameSite = http.SameSiteNoneMode
	}
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: false,
		Secure:   secure,
		SameSite: sameSite,
		Expires:  expiresAt,
	})
}

func clearCSRFCookie(w http.ResponseWriter, secure bool) {
	setCSRFCookie(w, secure, "", time.Unix(0, 0))
}
