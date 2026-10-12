package web

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"trackharbor/internal/store"
)

const sessionCookie = "__Host-trackharbor"
const sessionLifetime = 7 * 24 * time.Hour

type authContextKey struct{}
type authenticatedUser struct {
	store.User
	SessionID string
}
type loginWindow struct {
	start time.Time
	count int
}
type authentication struct {
	db       *store.Store
	key      []byte
	mu       sync.Mutex
	attempts map[string]loginWindow
	workers  chan struct{}
}

func newAuthentication(db *store.Store) *authentication {
	key, _ := db.SigningKey()
	return &authentication{db: db, key: key, attempts: map[string]loginWindow{}, workers: make(chan struct{}, 4)}
}
func currentUser(r *http.Request) store.User {
	return r.Context().Value(authContextKey{}).(authenticatedUser).User
}
func userSourceKey(r *http.Request, source string) string {
	return fmt.Sprintf("%d:%s", currentUser(r).ID, source)
}

func (a *authentication) authenticate(r *http.Request) (store.User, string, error) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || len(a.key) < 32 || len(cookie.Value) > 4096 {
		return store.User{}, "", store.ErrAuthentication
	}
	claims := &jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(cookie.Value, claims, func(*jwt.Token) (any, error) { return a.key, nil }, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired(), jwt.WithIssuer("trackharbor"), jwt.WithAudience("trackharbor-web"), jwt.WithIssuedAt())
	if err != nil || !token.Valid || claims.ID == "" || claims.Subject == "" {
		return store.User{}, "", store.ErrAuthentication
	}
	u, err := a.db.SessionUser(claims.ID, time.Now())
	if err != nil {
		return store.User{}, "", err
	}
	if strconv.FormatInt(u.ID, 10) != claims.Subject {
		return store.User{}, "", store.ErrAuthentication
	}
	return u, claims.ID, nil
}

func (a *authentication) allowLogin(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	for k, v := range a.attempts {
		if now.Sub(v.start) >= time.Minute {
			delete(a.attempts, k)
		}
	}
	v := a.attempts[host]
	if v.start.IsZero() {
		if len(a.attempts) >= 1024 {
			return false
		}
		v.start = now
	}
	v.count++
	a.attempts[host] = v
	return v.count <= 20
}

var loginTemplate = template.Must(template.New("login").Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>TrackHarbor · Sign in</title><link rel="stylesheet" href="/assets/tailwind.css"><script src="/assets/theme.js"></script></head><body><main class="min-h-screen flex items-center justify-center p-4"><section class="panel w-full max-w-sm rounded-lg border border-edge bg-surface p-6"><h1>TrackHarbor</h1><h2>Sign in</h2>{{if .}}<p class="account-error" role="alert">{{.}}</p>{{end}}<form method="post" action="/login"><label>Username<input name="username" autocomplete="username" maxlength="64" required autofocus></label><label>Password<input type="password" name="password" autocomplete="current-password" required></label><button type="submit">Sign in</button></form></section></main></body></html>`))

func (s *Server) loginPage(w http.ResponseWriter, r *http.Request) { s.renderLogin(w, 200, "") }
func (s *Server) renderLogin(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := loginTemplate.Execute(w, message); err != nil {
		s.logger.Error("render login failed")
	}
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !s.auth.allowLogin(r.RemoteAddr) {
		w.Header().Set("Retry-After", "60")
		s.renderLogin(w, 429, "Too many attempts. Try again in a minute.")
		return
	}
	select {
	case s.auth.workers <- struct{}{}:
		defer func() { <-s.auth.workers }()
	default:
		s.renderLogin(w, 429, "Too many attempts. Try again shortly.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := r.ParseForm(); err != nil {
		s.renderLogin(w, 400, "Invalid login form.")
		return
	}
	u, err := s.store.Authenticate(r.PostForm.Get("username"), r.PostForm.Get("password"))
	if err != nil {
		s.renderLogin(w, 401, "Invalid username or password.")
		return
	}
	if len(s.auth.key) < 32 {
		http.Error(w, "authentication unavailable", 503)
		return
	}
	sid := make([]byte, 32)
	if _, err = rand.Read(sid); err != nil {
		http.Error(w, "could not sign in", 500)
		return
	}
	now := time.Now()
	expires := now.Add(sessionLifetime)
	id := hex.EncodeToString(sid)
	claims := jwt.RegisteredClaims{Issuer: "trackharbor", Subject: strconv.FormatInt(u.ID, 10), Audience: jwt.ClaimStrings{"trackharbor-web"}, ID: id, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(expires)}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.auth.key)
	if err != nil {
		http.Error(w, "could not sign in", 500)
		return
	}
	if err = s.store.CreateSession(u, id, expires); err != nil {
		s.renderLogin(w, 401, "Account changed. Sign in again.")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode, Expires: expires, MaxAge: int(sessionLifetime.Seconds())})
	http.Redirect(w, r, "/", 303)
}
func clearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
}

func redirectToLogin(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/login")
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	sid := r.Context().Value(authContextKey{}).(authenticatedUser).SessionID
	if err := s.store.RevokeSession(sid); err != nil {
		http.Error(w, "could not sign out", 500)
		return
	}
	clearSession(w)
	http.Redirect(w, r, "/login", 303)
}
func (s *Server) authStatus(w http.ResponseWriter, r *http.Request) {
	users, err := s.store.Users()
	if err != nil {
		http.Error(w, "unable to read account status", 500)
		return
	}
	bootstrap := false
	for _, u := range users {
		if u.Username == "admin" {
			bootstrap = true
		}
	}
	w.Header().Set("Content-Type", "application/json")
	if err = json.NewEncoder(w).Encode(struct {
		ID        int64  `json:"id"`
		Username  string `json:"username"`
		Bootstrap bool   `json:"bootstrap"`
	}{currentUser(r).ID, currentUser(r).Username, bootstrap}); err != nil {
		s.logger.Error("write account status failed")
	}
}
func parseUserForm(w http.ResponseWriter, r *http.Request) error {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	return r.ParseForm()
}
func userID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, store.ErrUserInput
	}
	return id, nil
}
func userError(w http.ResponseWriter, err error) {
	status := 500
	message := "Could not update user. Try again."
	switch {
	case errors.Is(err, store.ErrUserInput):
		status, message = 400, err.Error()
	case errors.Is(err, sql.ErrNoRows):
		status, message = 404, "User no longer exists. Refresh Settings."
	case errors.Is(err, store.ErrUserExists), errors.Is(err, store.ErrLastUser):
		status, message = 409, err.Error()
	}
	http.Error(w, message, status)
}
func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	if err := parseUserForm(w, r); err != nil {
		http.Error(w, "invalid form", 400)
		return
	}
	if _, err := s.store.CreateUser(r.PostForm.Get("username"), r.PostForm.Get("password")); err != nil {
		userError(w, err)
		return
	}
	http.Redirect(w, r, "/?source=settings", 303)
}
func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	if err := parseUserForm(w, r); err != nil {
		http.Error(w, "invalid form", 400)
		return
	}
	id, err := userID(r)
	if err != nil {
		userError(w, err)
		return
	}
	if err = s.store.SetPassword(id, r.PostForm.Get("password")); err != nil {
		userError(w, err)
		return
	}
	if id == currentUser(r).ID {
		clearSession(w)
		redirectToLogin(w, r)
		return
	}
	http.Redirect(w, r, "/?source=settings", 303)
}
func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	if err := parseUserForm(w, r); err != nil {
		http.Error(w, "invalid form", 400)
		return
	}
	if r.PostForm.Get("confirm") != "yes" {
		http.Error(w, "user deletion requires confirmation", 400)
		return
	}
	id, err := userID(r)
	if err != nil {
		userError(w, err)
		return
	}
	if err = s.store.DeleteUser(id); err != nil {
		userError(w, err)
		return
	}
	if id == currentUser(r).ID {
		clearSession(w)
		redirectToLogin(w, r)
		return
	}
	http.Redirect(w, r, "/?source=settings", 303)
}
