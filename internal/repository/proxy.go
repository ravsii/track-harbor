package repository

import (
	"context"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
)

type ProxyRepository struct {
	db *sqlx.DB
	mu sync.Mutex
}

func NewProxyRepository(db *sqlx.DB) (*ProxyRepository, error) {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS verified_proxies (
		url TEXT PRIMARY KEY,
		valid_until INTEGER NOT NULL
	)`)
	if err != nil {
		return nil, err
	}
	return &ProxyRepository{db: db}, nil
}

func (r *ProxyRepository) Valid(ctx context.Context, now time.Time) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.db.ExecContext(ctx, `DELETE FROM verified_proxies WHERE valid_until <= ?`, now.Unix()); err != nil {
		return nil, err
	}
	var addresses []string
	err := r.db.SelectContext(ctx, &addresses, `SELECT url FROM verified_proxies WHERE valid_until > ? ORDER BY url`, now.Unix())
	return addresses, err
}

func (r *ProxyRepository) Save(ctx context.Context, address string, until time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, err := r.db.ExecContext(ctx, `INSERT INTO verified_proxies (url,valid_until) VALUES (?,?)
		ON CONFLICT(url) DO UPDATE SET valid_until=excluded.valid_until`, address, until.Unix())
	return err
}
