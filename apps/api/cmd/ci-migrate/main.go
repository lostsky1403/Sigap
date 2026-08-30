package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sigap/sigap/apps/api/internal/migrate"
)

func run() error {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	dir, err := migrate.DefaultDir()
	if err != nil {
		return fmt.Errorf("find migration directory: %w", err)
	}

	count, err := migrate.Run(ctx, pool, dir)
	if err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	log.Printf("Applied %d migrations", count)
	return nil
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("[ci-migrate] ")

	if err := run(); err != nil {
		log.Fatal(err)
	}
}
