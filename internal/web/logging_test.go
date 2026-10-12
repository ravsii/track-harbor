package web

import (
	"net/http"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestAuthPollingDoesNotLogAndLoginNeverLogsSecrets(t *testing.T) {
	db, _, _ := webFixture(t)
	core, logs := observer.New(zap.InfoLevel)
	h := New(db, zap.New(core))
	c := login(t, h, "admin", "admin")
	logs.TakeAll()
	if w := request(h, http.MethodGet, "/auth/status", nil, c); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if logs.Len() != 0 {
		t.Fatal("account polling is logged")
	}
	if w := request(h, http.MethodPost, "/logout", nil, c); w.Code != 303 {
		t.Fatal(w.Code)
	}
	for _, entry := range logs.All() {
		for _, field := range entry.Context {
			if field.String == c.Value {
				t.Fatal("JWT in logs")
			}
		}
	}
}
