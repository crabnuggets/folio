package httpapi

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/crabnuggets/folio/services/api/internal/domain"
)

const chatGPTCookie = "folio_chatgpt_sign_in"

func (s *server) chatGPTRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/chatgpt/connection", func(w http.ResponseWriter, r *http.Request) {
		status, err := s.chatgpt.Connection(r.Context())
		if err != nil {
			writeError(w, r, err)
			return
		}
		writeJSON(w, r, 200, status, 0)
	})
	m.HandleFunc("POST /api/v1/chatgpt/connect", func(w http.ResponseWriter, r *http.Request) {
		if !chatGPTInput(w, r) {
			return
		}
		// The supported plan authorization callback reaches this browser's computer.
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" {
				writeError(w, r, domain.Fail(400, "CHATGPT_LOCAL_CALLBACK_REQUIRED", "Open Folio on http://127.0.0.1 to connect your ChatGPT plan."))
				return
			}
		}
		authorization, binding, err := s.chatgpt.Begin(r.Context())
		if err != nil {
			writeError(w, r, err)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: chatGPTCookie, Value: binding, Path: "/auth/chatgpt/callback", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 600})
		writeJSON(w, r, 200, map[string]string{"authorization_url": authorization}, 0)
	})
	m.HandleFunc("POST /api/v1/chatgpt/disconnect", func(w http.ResponseWriter, r *http.Request) {
		if !chatGPTInput(w, r) {
			return
		}
		if err := s.chatgpt.Disconnect(r.Context()); err != nil {
			writeError(w, r, err)
			return
		}
		status, err := s.chatgpt.Connection(r.Context())
		if err != nil {
			writeError(w, r, err)
			return
		}
		writeJSON(w, r, 200, status, 0)
	})
}

func chatGPTInput(w http.ResponseWriter, r *http.Request) bool {
	if strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/json" {
		writeError(w, r, domain.Fail(415, "UNSUPPORTED_MEDIA_TYPE", "Content-Type must be application/json"))
		return false
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1024))
	if err != nil {
		writeError(w, r, domain.Fail(413, "PAYLOAD_TOO_LARGE", "Request is too large"))
		return false
	}
	if _, err = readJSON[struct{}](body); err != nil {
		writeError(w, r, err)
		return false
	}
	return true
}

func (s *server) chatGPTCallback(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(chatGPTCookie)
	http.SetCookie(w, &http.Cookie{Name: chatGPTCookie, Value: "", Path: "/auth/chatgpt/callback", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0)})
	if err != nil {
		writeError(w, r, domain.Fail(400, "CHATGPT_SIGN_IN_FAILED", "ChatGPT sign-in could not be verified. Open Folio at http://127.0.0.1 and start again."))
		return
	}
	if err = s.chatgpt.Callback(r.Context(), r.URL.Query(), cookie.Value); err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'sha256-jqxtvDkBbRAl9Hpqv68WdNOieepg8tJSYu1xIy7zT34='; frame-ancestors 'none'")
	_, _ = io.WriteString(w, `<!doctype html><html lang="en"><meta charset="utf-8"><title>ChatGPT connected</title><h1>ChatGPT connected</h1><p>Return to Folio. You can close this window.</p><script>window.close()</script></html>`)
}
