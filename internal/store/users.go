package store

import (
	"crypto/rand"
	"database/sql"
	"errors"
	"regexp"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrAuthentication = errors.New("invalid login or expired session")
	ErrLastUser       = errors.New("cannot delete the last user")
	ErrUserExists     = errors.New("username already exists")
	ErrUserInput      = errors.New("username must be 1–64 letters, digits, dots, underscores or hyphens; password must be 1–72 bytes")
	usernamePattern   = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
)

type User struct {
	ID           int64
	Username     string
	PasswordHash string
}

func (s *Store) initAuth() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	key := make([]byte, 32)
	if _, err = rand.Read(key); err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT OR IGNORE INTO auth_meta(id,signing_key) VALUES(1,?)`, key); err != nil {
		return err
	}
	var done bool
	if err = tx.QueryRow(`SELECT bootstrapped FROM auth_meta WHERE id=1`).Scan(&done); err != nil {
		return err
	}
	if !done {
		hash, err := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(`INSERT INTO users(username,password_hash) VALUES('admin',?)`, string(hash)); err != nil {
			return err
		}
		if _, err = tx.Exec(`UPDATE auth_meta SET bootstrapped=1 WHERE id=1`); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SigningKey() ([]byte, error) {
	var key []byte
	err := s.db.QueryRow(`SELECT signing_key FROM auth_meta WHERE id=1`).Scan(&key)
	return key, err
}

func passwordHash(password string) (string, error) {
	if len(password) == 0 || len(password) > 72 {
		return "", ErrUserInput
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

func (s *Store) CreateUser(username, password string) (User, error) {
	if !usernamePattern.MatchString(username) {
		return User{}, ErrUserInput
	}
	hash, err := passwordHash(password)
	if err != nil {
		return User{}, err
	}
	result, err := s.db.Exec(`INSERT INTO users(username,password_hash) VALUES(?,?) ON CONFLICT(username) DO NOTHING`, username, hash)
	if err != nil {
		return User{}, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return User{}, err
	}
	if n == 0 {
		return User{}, ErrUserExists
	}
	id, err := result.LastInsertId()
	return User{ID: id, Username: username, PasswordHash: hash}, err
}

func (s *Store) Users() ([]User, error) {
	rows, err := s.db.Query(`SELECT id,username FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := []User{}
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *Store) Authenticate(username, password string) (User, error) {
	var u User
	err := s.db.QueryRow(`SELECT id,username,password_hash FROM users WHERE username=?`, username).Scan(&u.ID, &u.Username, &u.PasswordHash)
	if errors.Is(err, sql.ErrNoRows) {
		// Equalize missing-user and wrong-password work without exposing usernames.
		_ = bcrypt.CompareHashAndPassword([]byte("$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"), []byte(password))
		return User{}, ErrAuthentication
	}
	if err != nil {
		return User{}, err
	}
	if len(password) > 72 || bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return User{}, ErrAuthentication
	}
	return u, nil
}

func (s *Store) SetPassword(id int64, password string) error {
	hash, err := passwordHash(password)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE users SET password_hash=? WHERE id=?`, hash, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	if _, err = tx.Exec(`DELETE FROM sessions WHERE user_id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DeleteUser(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM users WHERE id=?`, id).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	if err = tx.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return err
	}
	if n <= 1 {
		return ErrLastUser
	}
	if _, err = tx.Exec(`DELETE FROM users WHERE id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) CreateSession(u User, id string, expires time.Time) error {
	result, err := s.db.Exec(`INSERT INTO sessions(id,user_id,expires_at) SELECT ?,id,? FROM users WHERE id=? AND password_hash=?`, id, expires.Unix(), u.ID, u.PasswordHash)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n == 0 {
		return ErrAuthentication
	}
	return err
}

func (s *Store) SessionUser(id string, now time.Time) (User, error) {
	var u User
	err := s.db.QueryRow(`SELECT u.id,u.username FROM users u JOIN sessions s ON s.user_id=u.id WHERE s.id=? AND s.expires_at>?`, id, now.Unix()).Scan(&u.ID, &u.Username)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrAuthentication
	}
	return u, err
}

func (s *Store) RevokeSession(id string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE id=?`, id)
	return err
}
