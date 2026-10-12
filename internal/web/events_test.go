package web

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
	"trackharbor/internal/config"
)

func TestRevokedSessionCannotReceiveMoreDownloadEvents(t *testing.T) {
	db, h, _ := webFixture(t)
	c := login(t, h, "admin", "admin")
	s := &Server{store: db, auth: newAuthentication(db), downloads: newDownloadJob(db, config.Config{}, zap.NewNop())}
	ts := httptest.NewServer(http.HandlerFunc(s.downloadEvents))
	defer ts.Close()
	r, err := http.NewRequest("GET", ts.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	r.AddCookie(c)
	response, err := (&http.Client{Timeout: time.Second}).Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	for range 2 {
		if _, err := reader.ReadString('\n'); err != nil {
			t.Fatal(err)
		}
	}
	var claims jwt.RegisteredClaims
	if _, _, err := jwt.NewParser().ParseUnverified(c.Value, &claims); err != nil {
		t.Fatal(err)
	}
	if err := db.RevokeSession(claims.ID); err != nil {
		t.Fatal(err)
	}
	s.downloads.publish(downloadState{ID: 1, Source: "spotify", SourceID: "id", Title: "New private event", Stage: "done"})
	line, _ := reader.ReadString('\n')
	if strings.HasPrefix(line, "data:") {
		t.Fatal("revoked JWT still received an event")
	}
}
