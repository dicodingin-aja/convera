// Command migrate membuat database dan menjalankan migrasi skema Convera.
//
// Pemakaian:
//
//	go run ./cmd/migrate createdb      # buat database dari DATABASE_URL bila belum ada
//	go run ./cmd/migrate up            # jalankan semua migrasi
//	go run ./cmd/migrate status        # lihat migrasi yang sudah/belum jalan
//	go run ./cmd/migrate down          # mundurkan satu migrasi
//	go run ./cmd/migrate reset         # mundurkan semua migrasi (HATI-HATI: menghapus semua tabel)
//	go run ./cmd/migrate create nama   # buat file migrasi SQL baru di folder migrations/
//
// DATABASE_URL dibaca dari environment, atau dari file .env di folder backend.
package main

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/dicodingin-aja/convera/backend/migrations"
)

func main() {
	log.SetFlags(0)
	if len(os.Args) < 2 {
		log.Fatal("perintah wajib diisi: createdb | up | status | down | reset | version | create <nama>")
	}
	loadDotEnv(".env")
	cmd, args := os.Args[1], os.Args[2:]

	if cmd == "create" {
		if len(args) < 1 {
			log.Fatal("nama migrasi wajib diisi, mis. create add_index_leads")
		}
		goose.SetSequential(true)
		if err := goose.Create(nil, "migrations", args[0], "sql"); err != nil {
			log.Fatal(err)
		}
		return
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL belum diisi (lihat .env.example)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if cmd == "createdb" {
		if err := createDB(ctx, dsn); err != nil {
			log.Fatal(err)
		}
		return
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("tidak bisa terhubung ke database: %v", err)
	}

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		log.Fatal(err)
	}
	if err := goose.RunContext(ctx, cmd, db, ".", args...); err != nil {
		log.Fatal(err)
	}
}

// createDB membuat database yang disebut di DATABASE_URL, dengan masuk ke database
// "postgres" memakai kredensial yang sama.
func createDB(ctx context.Context, dsn string) error {
	u, err := url.Parse(dsn)
	if err != nil {
		return fmt.Errorf("DATABASE_URL tidak valid: %w", err)
	}
	name := strings.TrimPrefix(u.Path, "/")
	if name == "" {
		return errors.New("nama database tidak ada di DATABASE_URL")
	}
	admin := *u
	admin.Path = "/postgres"

	conn, err := pgx.Connect(ctx, admin.String())
	if err != nil {
		return fmt.Errorf("tidak bisa terhubung ke server PostgreSQL: %w", err)
	}
	defer conn.Close(ctx)

	var exists bool
	if err := conn.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", name).Scan(&exists); err != nil {
		return err
	}
	if exists {
		fmt.Printf("database %q sudah ada\n", name)
		return nil
	}
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()+" ENCODING 'UTF8'"); err != nil {
		return fmt.Errorf("gagal membuat database: %w", err)
	}
	fmt.Printf("database %q dibuat\n", name)
	return nil
}

// loadDotEnv mengisi variabel dari file .env sederhana (KEY=VALUE) tanpa menimpa
// variabel yang sudah ada di environment.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"'`)
		if _, set := os.LookupEnv(k); !set {
			os.Setenv(k, v)
		}
	}
}
