package api

import (
	"errors"
	"net/http"
	"strings"

	"ashen/api/internal/auth"
	"ashen/api/internal/store"
)

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type tokenResponse struct {
	Token  string `json:"token"`
	UserID string `json:"user_id"`
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	in.Email = strings.TrimSpace(strings.ToLower(in.Email))
	if in.Email == "" || len(in.Password) < 8 {
		writeErr(w, http.StatusBadRequest, "email required, password min 8 chars")
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "hash failed")
		return
	}
	u, err := s.store.CreateUser(r.Context(), in.Email, hash)
	if err != nil {
		if strings.Contains(err.Error(), "users_email_key") {
			writeErr(w, http.StatusConflict, "email already registered")
			return
		}
		writeErr(w, http.StatusInternalServerError, "create user failed")
		return
	}
	tok, err := s.tokens.Issue(u.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "token failed")
		return
	}
	writeJSON(w, http.StatusCreated, tokenResponse{Token: tok, UserID: u.ID})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	in.Email = strings.TrimSpace(strings.ToLower(in.Email))
	u, err := s.store.UserByEmail(r.Context(), in.Email)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		writeErr(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if !auth.CheckPassword(u.PasswordHash, in.Password) {
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	tok, err := s.tokens.Issue(u.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "token failed")
		return
	}
	writeJSON(w, http.StatusOK, tokenResponse{Token: tok, UserID: u.ID})
}
