// worker mengirim event berstatus 'queued' ke Meta Conversions API. Jalankan: go run ./cmd/worker
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dicodingin-aja/convera/backend/internal/ads"
	"github.com/dicodingin-aja/convera/backend/internal/config"
	"github.com/dicodingin-aja/convera/backend/internal/dispatch"
	"github.com/dicodingin-aja/convera/backend/internal/jobs"
	"github.com/dicodingin-aja/convera/backend/internal/meta"
	"github.com/dicodingin-aja/convera/backend/internal/secure"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	cfg, err := config.Load(true) // worker wajib punya GRAPH_API_VERSION
	if err != nil {
		log.Fatalf("konfigurasi: %v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("database tidak terjangkau: %v", err)
	}
	box, err := secure.NewBox(cfg.EncryptKey)
	if err != nil {
		log.Fatal(err)
	}
	d := &dispatch.Dispatcher{DB: pool, Box: box, Meta: &meta.Client{BaseURL: cfg.GraphBaseURL, Version: cfg.GraphAPIVersion}}
	log.Printf("worker berjalan (Graph API %s)", cfg.GraphAPIVersion)
	go dispatch.RunExpiry(ctx, pool, 5*time.Minute)
	syncer := &ads.Syncer{DB: pool, Box: box, Client: &ads.Client{BaseURL: cfg.GraphBaseURL, Version: cfg.GraphAPIVersion}}
	go jobs.Run(ctx, pool, syncer.SyncAll) // rekap harian, pengingat, retensi, sinkronisasi Meta Ads
	d.Run(ctx, 3*time.Second)
	log.Print("worker berhenti")
}
