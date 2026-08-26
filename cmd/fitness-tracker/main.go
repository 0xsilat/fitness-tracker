package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/local/fitness-tracker/internal/store"
	webapp "github.com/local/fitness-tracker/internal/web"
)

func main() {
	ctx := context.Background()
	databaseURL := env("DATABASE_URL", "postgres://fitness:fitness@localhost:5432/fitness?sslmode=disable")
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil { log.Fatal(err) }
	defer pool.Close()

	for i := 0; i < 30; i++ {
		if err = pool.Ping(ctx); err == nil { break }
		log.Printf("waiting for database: %v", err)
		time.Sleep(time.Second)
	}
	if err != nil { log.Fatalf("database unavailable: %v", err) }

	db := store.New(pool)
	if err := db.Migrate(ctx); err != nil { log.Fatalf("migrate: %v", err) }

	server := &http.Server{
		Addr: env("HTTP_ADDR", ":8080"), Handler: webapp.New(db),
		ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second,
	}
	go func() {
		log.Printf("fitness tracker listening on %s", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed { log.Fatal(err) }
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdown)
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" { return value }
	return fallback
}

