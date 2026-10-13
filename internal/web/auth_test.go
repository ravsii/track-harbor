package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
	"trackharbor/internal/config"
	"trackharbor/internal/store"
)

func webFixture(t *testing.T) (*store.Store, http.Handler, string) {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.InitDownloadSettings(store.DownloadSettings{Strategy: []string{"direct"}}); err != nil {
		t.Fatal(err)
	}
	if err = db.SetDownloadsStopped(true); err != nil {
		t.Fatal(err)
	}
	return db, New(db, zap.NewNop(), config.Config{Listen: "127.0.0.1:8787", DownloadDir: dir}), dir
}
func request(h http.Handler, method, path string, body url.Values, cookie *http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://localhost"+path, strings.NewReader(body.Encode()))
	if method == "POST" {
		r.Header.Set("Origin", "http://localhost")
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if cookie != nil {
		r.AddCookie(cookie)
		if method == "POST" {
			var claims jwt.RegisteredClaims
			if _, _, err := jwt.NewParser().ParseUnverified(cookie.Value, &claims); err == nil {
				r.Header.Set("X-TrackHarbor-User", claims.Subject)
			}
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func login(t *testing.T, h http.Handler, user, password string) *http.Cookie {
	t.Helper()
	w := request(h, "POST", "/login", url.Values{"username": {user}, "password": {password}}, nil)
	if w.Code != 303 || len(w.Result().Cookies()) == 0 {
		t.Fatalf("login: %d %s", w.Code, w.Body.String())
	}
	return w.Result().Cookies()[0]
}
func TestAuthenticationProtectsAllPrivateEndpoints(t *testing.T) {
	_, h, _ := webFixture(t)
	for _, path := range []string{"/", "/audio?id=1", "/audio/tracks", "/download/status", "/status?source=vk", "/ping-status", "/download/events", "/settings/personal/1/address"} {
		w := request(h, "GET", path, nil, nil)
		if w.Code != 401 {
			t.Errorf("%s: want 401, got %d", path, w.Code)
		}
	}
	for _, path := range []string{"/connect", "/sync", "/settings", "/settings/users", "/download/stop"} {
		w := request(h, "POST", path, nil, nil)
		if w.Code != 401 {
			t.Errorf("%s: want 401, got %d", path, w.Code)
		}
	}
	if w := request(h, "GET", "/login", nil, nil); w.Code != 200 || !strings.Contains(w.Body.String(), `autocomplete="current-password"`) {
		t.Fatalf("login form: %d", w.Code)
	}
	c := login(t, h, "admin", "admin")
	if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode || c.MaxAge != 7*24*60*60 {
		t.Fatalf("insecure cookie: %+v", c)
	}
	if w := request(h, "GET", "/", nil, c); w.Code != 200 || !strings.Contains(w.Body.String(), "Создайте отдельного пользователя") {
		t.Fatalf("bootstrap warning: %d %s", w.Code, w.Body.String())
	}
}
func TestLogoutPasswordResetAndDeleteRevokeJWT(t *testing.T) {
	db, h, _ := webFixture(t)
	u, err := db.CreateUser("alice", "password")
	if err != nil {
		t.Fatal(err)
	}
	a := login(t, h, "alice", "password")
	b := login(t, h, "alice", "password")
	if w := request(h, "POST", "/logout", nil, a); w.Code != 303 {
		t.Fatal(w.Code)
	}
	if w := request(h, "GET", "/", nil, a); w.Code != 401 {
		t.Fatal("logged out JWT reused")
	}
	if w := request(h, "GET", "/", nil, b); w.Code != 200 {
		t.Fatal("logout revoked another login")
	}
	if err = db.SetPassword(u.ID, "new-password"); err != nil {
		t.Fatal(err)
	}
	if w := request(h, "GET", "/", nil, b); w.Code != 401 {
		t.Fatal("reset JWT valid")
	}
	b = login(t, h, "alice", "new-password")
	if err = db.DeleteUser(u.ID); err != nil {
		t.Fatal(err)
	}
	if w := request(h, "GET", "/", nil, b); w.Code != 401 {
		t.Fatal("deleted user JWT valid")
	}
}
func TestJWTRejectsTamperingMissingClaimsAndExpiration(t *testing.T) {
	db, h, _ := webFixture(t)
	c := login(t, h, "admin", "admin")
	key, err := db.SigningKey()
	if err != nil {
		t.Fatal(err)
	}
	var issued jwt.RegisteredClaims
	if _, err := jwt.ParseWithClaims(c.Value, &issued, func(*jwt.Token) (any, error) { return key, nil }); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*jwt.RegisteredClaims){
		func(c *jwt.RegisteredClaims) { c.ExpiresAt = nil },
		func(c *jwt.RegisteredClaims) { c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Hour)) },
		func(c *jwt.RegisteredClaims) { c.Issuer = "other" },
		func(c *jwt.RegisteredClaims) { c.Subject = "999" },
		func(c *jwt.RegisteredClaims) { c.Audience = jwt.ClaimStrings{"other"} },
		func(c *jwt.RegisteredClaims) { c.ID = "nonexistent-session" },
		func(c *jwt.RegisteredClaims) { c.IssuedAt = jwt.NewNumericDate(time.Now().Add(time.Hour)) },
	} {
		claims := issued
		mutate(&claims)
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		bad := *c
		bad.Value = token
		if w := request(h, "GET", "/", nil, &bad); w.Code != 401 {
			t.Fatalf("claims accepted: %d", w.Code)
		}
	}
	wrongAlgorithm, err := jwt.NewWithClaims(jwt.SigningMethodHS384, issued).SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	badMethod := *c
	badMethod.Value = wrongAlgorithm
	if w := request(h, "GET", "/", nil, &badMethod); w.Code != 401 {
		t.Fatal("unexpected JWT algorithm accepted")
	}
	bad := *c
	bad.Value = "x" + c.Value
	if w := request(h, "GET", "/", nil, &bad); w.Code != 401 {
		t.Fatal("tampered JWT accepted")
	}
}
func TestHTTPSOriginAndCSRF(t *testing.T) {
	_, h, _ := webFixture(t)
	c := login(t, h, "admin", "admin")
	for _, origin := range []string{"", "https://evil.example", "null"} {
		r := httptest.NewRequest("POST", "https://music.example/settings/users", strings.NewReader("username=bad&password=pass"))
		r.Header.Set("Origin", origin)
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.AddCookie(c)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("CSRF %q: %d", origin, w.Code)
		}
	}
	r := httptest.NewRequest("POST", "http://music.example/settings/users", strings.NewReader("username=alice&password=pass"))
	r.Header.Set("Origin", "https://music.example")
	r.Header.Set("X-TrackHarbor-User", "1")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(c)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 303 {
		t.Fatalf("HTTPS behind proxy: %d %s", w.Code, w.Body.String())
	}
}

func TestStaleAccountMutationCannotSaveCredentialsToAnotherAccount(t *testing.T) {
	db, h, _ := webFixture(t)
	bob, err := db.CreateUser("bob", "password")
	if err != nil {
		t.Fatal(err)
	}
	c := login(t, h, "bob", "password")
	r := httptest.NewRequest("GET", "http://localhost/?source=spotify", nil)
	r.Header.Set("HX-Request", "true")
	r.Header.Set("X-TrackHarbor-User", "1")
	r.AddCookie(c)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 409 {
		t.Fatal("HTMX swapped another account's form into a stale page")
	}
	for _, header := range []string{"", "1"} {
		r := httptest.NewRequest("POST", "http://localhost/connect", strings.NewReader("source=spotify&headers=authorization%3A+Bearer+alice%0Aclient-token%3A+alice-client"))
		r.Header.Set("Origin", "http://localhost")
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("X-TrackHarbor-User", header)
		r.AddCookie(c)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 409 {
			t.Fatalf("stale account %q accepted: %d", header, w.Code)
		}
		connected, err := db.Connected(bob.ID, "spotify")
		if err != nil || connected {
			t.Fatal("Alice's credentials saved to Bob")
		}
	}
}

func TestInvalidUserRequestsReturnActionableErrors(t *testing.T) {
	_, h, _ := webFixture(t)
	c := login(t, h, "admin", "admin")
	for _, tt := range []struct {
		path   string
		form   url.Values
		status int
	}{
		{"/settings/users", url.Values{"username": {"has spaces"}, "password": {"pass"}}, 400},
		{"/settings/users", url.Values{"username": {"valid"}, "password": {""}}, 400},
		{"/settings/users", url.Values{"username": {"valid"}, "password": {strings.Repeat("é", 37)}}, 400},
		{"/settings/users", url.Values{"username": {"admin"}, "password": {"pass"}}, 409},
		{"/settings/users/999/password", url.Values{"password": {"pass"}}, 404},
		{"/settings/users/999/delete", url.Values{"confirm": {"yes"}}, 404},
		{"/settings/users/not-an-id/delete", url.Values{"confirm": {"yes"}}, 400},
	} {
		w := request(h, "POST", tt.path, tt.form, c)
		if w.Code != tt.status {
			t.Errorf("%s: want %d got %d: %s", tt.path, tt.status, w.Code, w.Body.String())
		}
	}
}

func TestLoginRateLimitAndRequestBodyBound(t *testing.T) {
	_, h, _ := webFixture(t)
	w := request(h, "POST", "/login", url.Values{"username": {"admin"}, "password": {strings.Repeat("x", 4096)}}, nil)
	if w.Code != 400 || len(w.Result().Cookies()) != 0 {
		t.Fatal("oversized login accepted")
	}
	for i := range 19 {
		w = request(h, "POST", "/login", nil, nil)
		if w.Code != 401 || len(w.Result().Cookies()) != 0 {
			t.Fatalf("wrong login attempt %d: %d", i, w.Code)
		}
	}
	w = request(h, "POST", "/login", url.Values{"username": {"admin"}, "password": {"admin"}}, nil)
	if w.Code != 429 || w.Header().Get("Retry-After") == "" || len(w.Result().Cookies()) != 0 {
		t.Fatal("login limiter bypassed")
	}
}
