// adminctl mengelola akun Convera Admin dari command line (tidak ada pendaftaran admin lewat web).
//
//	go run ./cmd/adminctl create <username> [nama] [superadmin|admin]
//	go run ./cmd/adminctl passwd <username>
//	go run ./cmd/adminctl disable <username>
//	go run ./cmd/adminctl list
//
// Password dibaca dari variabel ADMIN_PASSWORD, atau dari baris pertama stdin bila variabel itu kosong.
// Jangan menaruh password di argumen perintah (tersimpan di riwayat shell).
package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/dicodingin-aja/convera/backend/internal/config"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

const minPasswordLen = 12

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	config.LoadDotEnv(".env")
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL belum diisi (lihat .env.example)")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer conn.Close(ctx)

	switch os.Args[1] {
	case "create":
		if len(os.Args) < 3 {
			usage()
		}
		user := strings.ToLower(strings.TrimSpace(os.Args[2]))
		name, role := user, "superadmin"
		if len(os.Args) > 3 {
			name = os.Args[3]
		}
		if len(os.Args) > 4 {
			role = os.Args[4]
		}
		if role != "superadmin" && role != "admin" {
			log.Fatal("role harus superadmin atau admin")
		}
		if len(user) < 3 || len(user) > 40 {
			log.Fatal("username 3-40 karakter")
		}
		hash := hashed(readPassword())
		_, err := conn.Exec(ctx, `INSERT INTO admins (username, password_hash, name, role) VALUES ($1,$2,$3,$4)`, user, hash, name, role)
		check(err, "membuat admin (username sudah ada?)")
		fmt.Printf("admin %q (%s) dibuat\n", user, role)
	case "passwd":
		if len(os.Args) < 3 {
			usage()
		}
		hash := hashed(readPassword())
		tag, err := conn.Exec(ctx, `UPDATE admins SET password_hash = $2 WHERE username = $1`, strings.ToLower(os.Args[2]), hash)
		check(err, "mengubah password")
		if tag.RowsAffected() == 0 {
			log.Fatal("admin tidak ditemukan")
		}
		// Semua sesi lama admin ini dicabut.
		_, _ = conn.Exec(ctx, `UPDATE sessions SET revoked_at = now() WHERE revoked_at IS NULL AND admin_id = (SELECT id FROM admins WHERE username = $1)`, strings.ToLower(os.Args[2]))
		fmt.Println("password diubah; sesi lama dicabut")
	case "disable":
		if len(os.Args) < 3 {
			usage()
		}
		tag, err := conn.Exec(ctx, `UPDATE admins SET status = 'disabled' WHERE username = $1`, strings.ToLower(os.Args[2]))
		check(err, "menonaktifkan")
		if tag.RowsAffected() == 0 {
			log.Fatal("admin tidak ditemukan")
		}
		_, _ = conn.Exec(ctx, `UPDATE sessions SET revoked_at = now() WHERE revoked_at IS NULL AND admin_id = (SELECT id FROM admins WHERE username = $1)`, strings.ToLower(os.Args[2]))
		fmt.Println("admin dinonaktifkan; sesi dicabut")
	case "list":
		rows, err := conn.Query(ctx, `SELECT username::text, name, role, status, last_login_at FROM admins ORDER BY created_at`)
		check(err, "membaca admin")
		defer rows.Close()
		for rows.Next() {
			var u, n, r, s string
			var last *string
			var t any
			check(rows.Scan(&u, &n, &r, &s, &t), "membaca baris")
			if t != nil {
				x := fmt.Sprint(t)
				last = &x
			}
			ll := "belum pernah login"
			if last != nil {
				ll = *last
			}
			fmt.Printf("%-20s %-12s %-9s %s\n", u, r, s, ll)
		}
	default:
		usage()
	}
}

func readPassword() string {
	p := os.Getenv("ADMIN_PASSWORD")
	if p == "" {
		fmt.Fprint(os.Stderr, "Password (min 12 karakter, akan terlihat saat diketik): ")
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		p = strings.TrimRight(line, "\r\n")
	}
	if len(p) < minPasswordLen {
		log.Fatalf("password minimal %d karakter", minPasswordLen)
	}
	if len(p) > 72 {
		log.Fatal("password maksimal 72 karakter (batas bcrypt)")
	}
	return p
}

func hashed(p string) string {
	h, err := bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
	check(err, "hash password")
	return string(h)
}

func check(err error, what string) {
	if err != nil {
		log.Fatalf("%s: %v", what, err)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "pemakaian: adminctl create <username> [nama] [superadmin|admin] | passwd <username> | disable <username> | list")
	os.Exit(2)
}
