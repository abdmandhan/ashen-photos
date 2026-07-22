package api

import (
	"context"
	"net/http"
	"strings"
)

type ctxKey string

const userIDKey ctxKey = "userID"

// authMiddleware requires a valid Bearer JWT and injects the user id into the context.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			writeErr(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		userID, err := s.tokens.Parse(strings.TrimPrefix(h, "Bearer "))
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "invalid token")
			return
		}
		ctx := context.WithValue(r.Context(), userIDKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func userID(r *http.Request) string {
	v, _ := r.Context().Value(userIDKey).(string)
	return v
}
