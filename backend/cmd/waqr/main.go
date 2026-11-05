// Command waqr: layanan sesi WhatsApp Web untuk mode QR. Hanya dipanggil API Convera (jaringan internal),
// dengan header X-Service-Token. Jangan diekspos ke internet.
package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/dicodingin-aja/convera/backend/internal/waqr"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
)

var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func main() {
	dsn, token, api := os.Getenv("DATABASE_URL"), os.Getenv("QR_SERVICE_TOKEN"), os.Getenv("CONVERA_API_URL")
	addr := os.Getenv("QR_SERVICE_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8090"
	}
	if dsn == "" || len(token) < 16 || api == "" {
		log.Fatal("DATABASE_URL, QR_SERVICE_TOKEN (min. 16 karakter), dan CONVERA_API_URL wajib diisi")
	}
	// Nama perangkat yang tampil di WhatsApp > Perangkat tertaut (hanya berlaku untuk penautan baru).
	name := os.Getenv("QR_DEVICE_NAME")
	if name == "" {
		name = "Convera"
	}
	store.SetOSInfo(name, [3]uint32{1, 0, 0})
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	// Penyimpanan kunci sesi WhatsApp (tabel whatsmeow_*) dibuat otomatis di database yang sama.
	store, err := sqlstore.New(ctx, "pgx", dsn, waLog.Stdout("store", "WARN", false))
	if err != nil {
		log.Fatalf("store sesi: %v", err)
	}
	m := waqr.NewManager(ctx, pool, store, api, token)
	go m.Resume(ctx)

	mux := http.NewServeMux()
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Service-Token")), []byte(token)) != 1 {
				http.Error(w, `{"message":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			if !uuidRe.MatchString(r.PathValue("ws")) {
				http.Error(w, `{"message":"workspace id tidak valid"}`, http.StatusBadRequest)
				return
			}
			h(w, r)
		}
	}
	reply := func(w http.ResponseWriter, code int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { reply(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("POST /sessions/{ws}/start", auth(func(w http.ResponseWriter, r *http.Request) {
		qr, st, err := m.Start(r.Context(), r.PathValue("ws"))
		if err != nil {
			log.Printf("start %s: %v", r.PathValue("ws"), err)
			reply(w, 503, map[string]string{"message": err.Error()})
			return
		}
		reply(w, 200, map[string]any{"qr": qr, "qr_image": waqr.QRImage(qr), "status": st})
	}))
	mux.HandleFunc("GET /sessions/{ws}", auth(func(w http.ResponseWriter, r *http.Request) {
		qr, st := m.Status(r.PathValue("ws"))
		reply(w, 200, map[string]any{"qr": qr, "qr_image": waqr.QRImage(qr), "status": st})
	}))
	mux.HandleFunc("DELETE /sessions/{ws}", auth(func(w http.ResponseWriter, r *http.Request) {
		if err := m.Delete(r.Context(), r.PathValue("ws")); err != nil {
			reply(w, 500, map[string]string{"message": err.Error()})
			return
		}
		reply(w, 200, map[string]string{"status": "disconnected"})
	}))

	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		sc, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_ = srv.Shutdown(sc)
	}()
	log.Printf("waqr mendengarkan di %s", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) && !strings.Contains(err.Error(), "closed") {
		log.Fatal(err)
	}
}
