package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/shivang-16/orbit.api/internal/config"
)

type Client struct {
	db *sql.DB
}

func Open(cfg config.Postgres) (*Client, error) {
	db, err := sql.Open("pgx", cfg.DSN())

	maxOpen := cfg.MaxOpenConns
	if maxOpen < 1 {
		maxOpen = 10
	}
	maxIdle := cfg.MaxIdleConns
	
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(time.Hour)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()



	return &Client{db: db}, nil
}

func OpenAndMigrate(cfg config.Postgres, migrationsDir string) (*Client, error) {
	client, err := Open(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Migrate(ctx, migrationsDir); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

func (c *Client) Ping(ctx context.Context) error {
	return c.db.PingContext(ctx)
}

func (c *Client) DB() *sql.DB {
	return c.db
}

func (c *Client) Close() error {
	return c.db.Close()
}
