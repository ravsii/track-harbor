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

// Changed when sources or verification policy change, so an older pool cannot
// bypass discovery or the new one-second check on the next download startup.
const proxyCachePolicy = "proxyscrape+proxifly/1s/v1"

func NewProxyRepository(db *sqlx.DB) (*ProxyRepository, error) {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS verified_proxies (
		url TEXT PRIMARY KEY,
		valid_until INTEGER NOT NULL
	)`)
	if err != nil {
		return nil, err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS proxy_cache_state (id INTEGER PRIMARY KEY CHECK(id=1), full_valid_until INTEGER NOT NULL)`)
	if err != nil {
		return nil, err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS proxy_cache_policy (id INTEGER PRIMARY KEY CHECK(id=1), policy TEXT NOT NULL)`)
	if err != nil {
		return nil, err
	}
	return &ProxyRepository{db: db}, nil
}

func (r *ProxyRepository) Ready(ctx context.Context, now time.Time) (bool, error) {
	var count int
	err := r.db.GetContext(ctx, &count, `SELECT COUNT(*) FROM proxy_cache_state s JOIN proxy_cache_policy p ON p.id=s.id WHERE s.id=1 AND s.full_valid_until > ? AND p.policy=?`, now.Unix(), proxyCachePolicy)
	return count > 0, err
}

func (r *ProxyRepository) Complete(ctx context.Context, until time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO proxy_cache_state (id,full_valid_until) VALUES (1,?) ON CONFLICT(id) DO UPDATE SET full_valid_until=excluded.full_valid_until`, until.Unix()); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO proxy_cache_policy (id,policy) VALUES (1,?) ON CONFLICT(id) DO UPDATE SET policy=excluded.policy`, proxyCachePolicy); err != nil {
		return err
	}
	return tx.Commit()
}

// Reset invalidates only the proxy cache; track history and library stay intact.
// Called after successfully fetching all pages, before saving new probes.
func (r *ProxyRepository) Reset(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM proxy_cache_state`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM proxy_cache_policy`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM verified_proxies`); err != nil {
		return err
	}
	return tx.Commit()
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
