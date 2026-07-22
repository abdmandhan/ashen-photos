package api

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"strings"
)

// errBodyRecorder captures the status code and (bounded) body so failures can be logged.
type errBodyRecorder struct {
	http.ResponseWriter
	status int
	buf    bytes.Buffer
}

func (r *errBodyRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *errBodyRecorder) Write(b []byte) (int, error) {
	if r.status >= 400 && r.buf.Len() < 1024 {
		r.buf.Write(b)
	}
	return r.ResponseWriter.Write(b)
}

// errorLogger logs the method, path, status, and response body for any 4xx/5xx.
// This surfaces the reason a request failed (e.g. bad media_type, presign error).
func errorLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &errBodyRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if rec.status >= 400 {
			log.Printf("ERROR %d %s %s: %s", rec.status, r.Method, r.URL.Path,
				strings.TrimSpace(rec.buf.String()))
		}
	})
}

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

// cors permits browser dashboards (any origin) to call the API. Dev-grade;
// tighten to an allowlist before production.
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func userID(r *http.Request) string {
	v, _ := r.Context().Value(userIDKey).(string)
	return v
}
