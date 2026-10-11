package main

import (
	"log"
	"net/http"
	"time"

	"trackharbor/server/internal/config"
	"trackharbor/server/internal/store"
	"trackharbor/server/internal/web"
)

func main() {
	cfg, dbPath, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	db, err := store.Open(dbPath)
	if err != nil {
		log.Fatal("open server database: ", err)
	}
	defer db.Close()
	srv := &http.Server{
		Addr: cfg.Listen, Handler: web.New(db),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second,
		WriteTimeout: 30 * time.Second, IdleTimeout: time.Minute,
	}
	log.Printf("TrackHarbor web preview: http://%s (local access only)", cfg.Listen)
	log.Fatal(srv.ListenAndServe())
}
