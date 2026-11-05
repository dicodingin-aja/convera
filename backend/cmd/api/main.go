// api menjalankan REST API Convera (Gin). Jalankan dari folder backend/: go run ./cmd/api
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dicodingin-aja/convera/backend/internal/config"
	"github.com/dicodingin-aja/convera/backend/internal/secure"
	"github.com/dicodingin-aja/convera/backend/internal/server"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	// GRAPH_API_VERSION hanya wajib untuk tombol Tes Koneksi; API tetap bisa jalan tanpanya.
	cfg, err := config.Load(false)
	if err != nil {
		log.Fatalf("konfigurasi: %v", err)
	}
	if cfg.GraphAPIVersion == "" {
		log.Print("peringatan: GRAPH_API_VERSION kosong, Tes Koneksi Meta tidak akan berfungsi")
	}
	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("database tidak terjangkau (sudah menjalankan 'go run ./cmd/migrate up'?): %v", err)
	}
	box, err := secure.NewBox(cfg.EncryptKey)
	if err != nil {
		log.Fatal(err)
	}

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           server.New(cfg, pool, box).Router(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
	}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	log.Printf("API berjalan di %s", cfg.Addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
