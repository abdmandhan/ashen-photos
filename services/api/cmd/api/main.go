package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ashen/api/internal/api"
	"ashen/api/internal/auth"
	"ashen/api/internal/config"
	"ashen/api/internal/db"
	"ashen/api/internal/queue"
	"ashen/api/internal/storage"
	"ashen/api/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx := context.Background()
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	// `api migrate` runs migrations then exits.
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		log.Println("migrations up to date")
		return
	}

	stg, err := storage.New(cfg.S3)
	if err != nil {
		log.Fatalf("storage init: %v", err)
	}

	q, err := queue.New(cfg.RedisURL)
	if err != nil {
		log.Fatalf("queue init: %v", err)
	}
	if err := q.Ping(ctx); err != nil {
		log.Fatalf("redis ping: %v", err)
	}

	srv := api.NewServer(store.New(pool), auth.NewTokenService(cfg.JWTSecret), stg, q)
	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("api listening on %s", cfg.Addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("serve: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutCtx)
	log.Println("stopped")
}
