package config

import (
	"context"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

func NewPool() (*pgxpool.Pool, error) {
	url := os.Getenv("DATABASE_URL")
	return pgxpool.New(context.Background(), url)
}
