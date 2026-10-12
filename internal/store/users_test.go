package store

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestBootstrapAndUserLifecycle(t *testing.T) {
	s := testStore(t)
	admin, err := s.Authenticate("admin", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Authenticate("admin", "wrong"); !errors.Is(err, ErrAuthentication) {
		t.Fatalf("wrong password: %v", err)
	}
	if err = s.DeleteUser(admin.ID); !errors.Is(err, ErrLastUser) {
		t.Fatalf("last user: %v", err)
	}
	u, err := s.CreateUser("alice", "password-one")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateUser("alice", "other"); !errors.Is(err, ErrUserExists) {
		t.Fatalf("duplicate: %v", err)
	}
	if err = s.CreateSession(u, "first", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err = s.CreateSession(u, "second", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err = s.SetPassword(u.ID, "password-two"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SessionUser("first", time.Now()); !errors.Is(err, ErrAuthentication) {
		t.Fatal("password reset did not revoke session")
	}
	if err = s.CreateSession(u, "stale-login", time.Now().Add(time.Hour)); !errors.Is(err, ErrAuthentication) {
		t.Fatal("old password login raced reset")
	}
	if _, err = s.Authenticate("alice", "password-one"); !errors.Is(err, ErrAuthentication) {
		t.Fatal("old password still accepted")
	}
	if _, err = s.Authenticate("alice", "password-two"); err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteUser(admin.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.initAuth(); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Authenticate("admin", "admin"); !errors.Is(err, ErrAuthentication) {
		t.Fatal("deleted bootstrap account recreated")
	}
}

func TestSessionLogoutExpiryAndDeletion(t *testing.T) {
	s := testStore(t)
	u, _ := s.CreateUser("bob", "password")
	for _, id := range []string{"logout", "keep"} {
		if err := s.CreateSession(u, id, time.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.RevokeSession("logout"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SessionUser("logout", time.Now()); !errors.Is(err, ErrAuthentication) {
		t.Fatal("logout session valid")
	}
	if _, err := s.SessionUser("keep", time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SessionUser("keep", time.Now().Add(2*time.Hour)); !errors.Is(err, ErrAuthentication) {
		t.Fatal("expired session valid")
	}
	if err := s.DeleteUser(u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SessionUser("keep", time.Now()); !errors.Is(err, ErrAuthentication) {
		t.Fatal("deleted user session valid")
	}
}

func TestUserValidationAndStableSigningKey(t *testing.T) {
	s := testStore(t)
	for _, name := range []string{"", "with space", "../path", "<script>"} {
		if _, err := s.CreateUser(name, "password"); !errors.Is(err, ErrUserInput) {
			t.Fatalf("accepted %q", name)
		}
	}
	if _, err := s.CreateUser("empty", ""); !errors.Is(err, ErrUserInput) {
		t.Fatal("empty password")
	}
	a, err := s.SigningKey()
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.SigningKey()
	if err != nil {
		t.Fatal(err)
	}
	if len(a) < 32 || string(a) != string(b) {
		t.Fatal("unstable signing key")
	}
}
