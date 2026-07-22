package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"ashen/api/internal/auth"
	"ashen/api/internal/queue"
	"ashen/api/internal/storage"
	"ashen/api/internal/store"
)

type Server struct {
	store   *store.Store
	tokens  *auth.TokenService
	storage *storage.Storage
	queue   *queue.Queue
}

func NewServer(st *store.Store, tokens *auth.TokenService, stg *storage.Storage, q *queue.Queue) *Server {
	return &Server{store: st, tokens: tokens, storage: stg, queue: q}
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(cors)
	r.Use(errorLogger)

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Route("/auth", func(r chi.Router) {
		r.Post("/register", s.handleRegister)
		r.Post("/login", s.handleLogin)
	})

	r.Group(func(r chi.Router) {
		r.Use(s.authMiddleware)

		r.Post("/devices", s.handleCreateDevice)
		r.Get("/devices", s.handleListDevices)

		r.Post("/uploads/check", s.handleUploadCheck)
		r.Post("/uploads", s.handleCreateUpload)
		r.Post("/uploads/{id}/complete", s.handleCompleteUpload)

		r.Get("/assets", s.handleListAssets)
		r.Get("/stats", s.handleStats)
	})

	return r
}
