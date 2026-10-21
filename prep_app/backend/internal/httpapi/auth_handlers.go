package httpapi

import (
	"errors"
	"net/http"
	"time"

	"prepapp/internal/auth"
)

func (s *Server) authStatus(w http.ResponseWriter, r *http.Request) {
	if s.Guard == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"password_set":  false,
			"authenticated": true,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"password_set":  s.Guard.PasswordSet(),
		"authenticated": s.Guard.Authenticate(tokenFrom(r)),
	})
}

func (s *Server) authLogin(w http.ResponseWriter, r *http.Request) {
	if s.Guard == nil {
		writeErr(w, http.StatusBadRequest, auth.ErrNoPassword)
		return
	}
	if !requireClientHeader(w, r) {
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	token, exp, err := s.Guard.Login(clientIP(r), body.Password)
	if err != nil {
		writeAuthErr(w, err)
		return
	}
	setSessionCookie(w, r, s.TrustProxy, token, int(time.Until(exp).Seconds()))
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"token":   token,
		"expires": exp.UTC().Format(time.RFC3339),
	})
}

func (s *Server) authSetup(w http.ResponseWriter, r *http.Request) {
	if s.Guard == nil {
		writeErr(w, http.StatusBadRequest, errors.New("защита не настроена"))
		return
	}
	if !isLoopback(r) {
		writeErr(w, http.StatusForbidden, errors.New("пароль задаётся только с компьютера, где запущен сервер"))
		return
	}
	if !requireClientHeader(w, r) {
		return
	}
	var body struct {
		Password string `json:"password"`
		Confirm  string `json:"confirm"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if body.Password != body.Confirm {
		writeErr(w, http.StatusBadRequest, errors.New("пароли не совпадают"))
		return
	}
	if err := s.Guard.EnablePassword(clientIP(r), body.Password); err != nil {
		writeAuthErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) authLogout(w http.ResponseWriter, r *http.Request) {
	if s.Guard != nil {
		s.Guard.Logout(tokenFrom(r))
	}
	clearSessionCookie(w, r, s.TrustProxy)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) authChangePassword(w http.ResponseWriter, r *http.Request) {
	if s.Guard == nil {
		writeErr(w, http.StatusBadRequest, auth.ErrNoPassword)
		return
	}
	if !requireClientHeader(w, r) {
		return
	}
	var body struct {
		Current  string `json:"current_password"`
		Password string `json:"password"`
		Confirm  string `json:"confirm"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if body.Password != body.Confirm {
		writeErr(w, http.StatusBadRequest, errors.New("пароли не совпадают"))
		return
	}
	if err := s.Guard.ChangePassword(clientIP(r), body.Current, body.Password); err != nil {
		writeAuthErr(w, err)
		return
	}
	clearSessionCookie(w, r, s.TrustProxy)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "reauth": true})
}
