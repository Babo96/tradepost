package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/babo96/tradepost/server/internal/api"
	"github.com/babo96/tradepost/server/internal/store"
	"github.com/babo96/tradepost/server/internal/worker"
)

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	databaseURL := env("TRADEPOST_DATABASE_URL", "postgres://tradepost:tradepost@localhost:5432/tradepost?sslmode=disable")
	st, err := store.NewPostgres(ctx, databaseURL)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()

	go func() {
		_ = st.CleanupHistory(ctx)
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := st.CleanupHistory(ctx); err != nil {
					log.Printf("history cleanup: %v", err)
				}
			}
		}
	}()

	// The worker runs in-process by default. Set TRADEPOST_WORKER=0 to run a
	// dedicated API-only replica; multiple worker replicas are safe.
	if env("TRADEPOST_WORKER", "1") != "0" {
		go worker.Run(ctx, st, time.Second)
	}

	addr := env("TRADEPOST_ADDR", ":8080")
	server := &http.Server{
		Addr:              addr,
		Handler:           api.New(st),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		log.Printf("shutting down")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	log.Printf("tradepostd listening on %s", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("http: %v", err)
	}
}
