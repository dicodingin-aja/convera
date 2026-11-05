// Package jobs berisi tugas terjadwal (bab 5.3): rekap harian, pengingat langganan & token Meta, retensi data.
// Semua tugas idempoten dan aman dijalankan ulang atau oleh lebih dari satu worker.
package jobs

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ---------- Rekap harian ----------

// RecomputeStats menghitung ulang daily_stats untuk daysBack hari terakhir (0 = hanya hari ini, zona Asia/Jakarta).
// workspaceID kosong = semua workspace. Hanya hari dalam rentang yang diganti, jadi data lama tidak tersentuh
// (itulah sebabnya retensi event minimal 60 hari: lebih panjang dari jendela backfill).
func RecomputeStats(ctx context.Context, db *pgxpool.Pool, daysBack int, workspaceID string) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	const rng = `(SELECT (now() AT TIME ZONE 'Asia/Jakarta')::date - $1::int) AND (SELECT (now() AT TIME ZONE 'Asia/Jakarta')::date)`
	if _, err := tx.Exec(ctx, `DELETE FROM daily_stats WHERE day BETWEEN `+rng+` AND ($2 = '' OR workspace_id::text = $2)`, daysBack, workspaceID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
WITH l AS (
  SELECT workspace_id, (first_message_at AT TIME ZONE 'Asia/Jakarta')::date AS day,
         count(*)::int AS leads, (count(*) FILTER (WHERE ctwa_clid IS NOT NULL))::int AS ads
  FROM leads WHERE deleted_at IS NULL AND ($2 = '' OR workspace_id::text = $2)
    AND (first_message_at AT TIME ZONE 'Asia/Jakarta')::date BETWEEN `+rng+` GROUP BY 1, 2),
e AS (
  SELECT workspace_id, (event_time AT TIME ZONE 'Asia/Jakarta')::date AS day,
         (count(*) FILTER (WHERE event_name = 'InitiateCheckout' AND status = 'sent'))::int AS checkouts,
         (count(*) FILTER (WHERE event_name = 'Purchase' AND status = 'sent'))::int AS purchases,
         COALESCE(sum(value) FILTER (WHERE event_name = 'Purchase' AND status = 'sent'), 0) AS pv
  FROM events WHERE source <> 'test' AND ($2 = '' OR workspace_id::text = $2)
    AND (event_time AT TIME ZONE 'Asia/Jakarta')::date BETWEEN `+rng+` GROUP BY 1, 2),
s AS (
  SELECT workspace_id, (created_at AT TIME ZONE 'Asia/Jakarta')::date AS day,
         (count(*) FILTER (WHERE status = 'sent'))::int AS sent, (count(*) FILTER (WHERE status = 'failed'))::int AS failed
  FROM events WHERE source <> 'test' AND ($2 = '' OR workspace_id::text = $2)
    AND (created_at AT TIME ZONE 'Asia/Jakarta')::date BETWEEN `+rng+` GROUP BY 1, 2),
k AS (SELECT workspace_id, day FROM l UNION SELECT workspace_id, day FROM e UNION SELECT workspace_id, day FROM s)
INSERT INTO daily_stats (workspace_id, day, chats_from_ads, leads, checkouts, purchases, purchase_value, events_sent, events_failed)
SELECT k.workspace_id, k.day, COALESCE(l.ads, 0), COALESCE(l.leads, 0), COALESCE(e.checkouts, 0), COALESCE(e.purchases, 0),
       COALESCE(e.pv, 0), COALESCE(s.sent, 0), COALESCE(s.failed, 0)
FROM k
JOIN workspaces w ON w.id = k.workspace_id AND w.deleted_at IS NULL
LEFT JOIN l ON l.workspace_id = k.workspace_id AND l.day = k.day
LEFT JOIN e ON e.workspace_id = k.workspace_id AND e.day = k.day
LEFT JOIN s ON s.workspace_id = k.workspace_id AND s.day = k.day`, daysBack, workspaceID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ---------- Pengingat ----------

type reminder struct {
	WorkspaceID, Workspace, Owner string
	Prefix                        string // sub | trial | token
	Ref                           time.Time
	Days                          int
}

// bucket menentukan tahap pengingat: H-1, H-3, atau H-7 (token: H-1 dan H-7). Hanya tahap yang berlaku SAAT INI
// yang dikirim, jadi worker yang mati beberapa hari tidak mengirim pengingat basi sekaligus.
func bucket(days int) int {
	switch {
	case days <= 1:
		return 1
	case days <= 3:
		return 3
	}
	return 7
}

func oneLine(s string) string {
	return strings.NewReplacer("\r", " ", "\n", " ", "\x00", "").Replace(s)
}

// SendReminders mengirim pengingat (notifikasi dalam aplikasi) dan menandai token yang sudah kedaluwarsa.
// Mengembalikan jumlah pengingat baru.
func SendReminders(ctx context.Context, db *pgxpool.Pool) (int, error) {
	rows, err := db.Query(ctx, `
SELECT w.id::text, w.name, u.name, 'sub', sub.current_period_end,
       ceil(extract(epoch FROM (sub.current_period_end - now())) / 86400)::int
FROM subscriptions sub JOIN workspaces w ON w.id = sub.workspace_id JOIN users u ON u.id = w.owner_user_id
WHERE w.deleted_at IS NULL AND u.deleted_at IS NULL AND sub.status = 'active'
  AND sub.current_period_end > now() AND sub.current_period_end <= now() + interval '7 days'
UNION ALL
SELECT w.id::text, w.name, u.name, 'trial', sub.trial_until,
       ceil(extract(epoch FROM (sub.trial_until - now())) / 86400)::int
FROM subscriptions sub JOIN workspaces w ON w.id = sub.workspace_id JOIN users u ON u.id = w.owner_user_id
WHERE w.deleted_at IS NULL AND u.deleted_at IS NULL AND sub.status = 'trial'
  AND sub.trial_until > now() AND sub.trial_until <= now() + interval '7 days'
UNION ALL
SELECT w.id::text, w.name, u.name, 'token', mc.token_expires_at,
       ceil(extract(epoch FROM (mc.token_expires_at - now())) / 86400)::int
FROM meta_connections mc JOIN workspaces w ON w.id = mc.workspace_id JOIN users u ON u.id = w.owner_user_id
WHERE w.deleted_at IS NULL AND u.deleted_at IS NULL AND mc.access_token_enc IS NOT NULL
  AND mc.token_expires_at > now() AND mc.token_expires_at <= now() + interval '7 days'`)
	if err != nil {
		return 0, err
	}
	var due []reminder
	for rows.Next() {
		var r reminder
		if err := rows.Scan(&r.WorkspaceID, &r.Workspace, &r.Owner, &r.Prefix, &r.Ref, &r.Days); err != nil {
			rows.Close()
			return 0, err
		}
		due = append(due, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	sent := 0
	for _, r := range due {
		b := bucket(r.Days)
		if r.Prefix == "token" && b == 3 {
			b = 7 // token: hanya H-7 dan H-1
		}
		kind := fmt.Sprintf("%s_%d", r.Prefix, b)
		tag, err := db.Exec(ctx, `INSERT INTO reminder_log (workspace_id, kind, ref_date) VALUES ($1::uuid, $2, $3) ON CONFLICT DO NOTHING`, r.WorkspaceID, kind, r.Ref)
		if err != nil {
			return sent, err
		}
		if tag.RowsAffected() == 0 {
			continue // sudah pernah dikirim untuk tanggal berakhir ini
		}
		title, body, link := reminderText(r)
		if _, err := db.Exec(ctx, `INSERT INTO notifications (user_id, workspace_id, type, title, body, link)
			SELECT owner_user_id, id, $2, $3, $4, $5 FROM workspaces WHERE id = $1::uuid`, r.WorkspaceID, kind, title, body, link); err != nil {
			return sent, err
		}
		sent++
	}

	// Token yang sudah lewat tanggal berlaku: tandai kedaluwarsa dan beri tahu sekali.
	exp, err := db.Query(ctx, `
SELECT w.id::text, w.name, u.name, mc.token_expires_at
FROM meta_connections mc JOIN workspaces w ON w.id = mc.workspace_id JOIN users u ON u.id = w.owner_user_id
WHERE w.deleted_at IS NULL AND u.deleted_at IS NULL AND mc.access_token_enc IS NOT NULL
  AND mc.token_expires_at IS NOT NULL AND mc.token_expires_at <= now()`)
	if err != nil {
		return sent, err
	}
	var gone []reminder
	for exp.Next() {
		var r reminder
		if err := exp.Scan(&r.WorkspaceID, &r.Workspace, &r.Owner, &r.Ref); err != nil {
			exp.Close()
			return sent, err
		}
		r.Prefix = "token"
		gone = append(gone, r)
	}
	exp.Close()
	for _, r := range gone {
		if _, err := db.Exec(ctx, `UPDATE meta_connections SET status = 'expired' WHERE workspace_id = $1::uuid AND status NOT IN ('expired', 'invalid')`, r.WorkspaceID); err != nil {
			return sent, err
		}
		tag, err := db.Exec(ctx, `INSERT INTO reminder_log (workspace_id, kind, ref_date) VALUES ($1::uuid, 'token_exp', $2) ON CONFLICT DO NOTHING`, r.WorkspaceID, r.Ref)
		if err != nil {
			return sent, err
		}
		if tag.RowsAffected() == 0 {
			continue
		}
		title, body, link := "Token Meta kedaluwarsa", "Token Conversions API untuk "+oneLine(r.Workspace)+" sudah kedaluwarsa, event berhenti terkirim ke Meta. Buat token baru di Events Manager lalu simpan di halaman Koneksi.", "/koneksi"
		if _, err := db.Exec(ctx, `INSERT INTO notifications (user_id, workspace_id, type, title, body, link)
			SELECT owner_user_id, id, 'token_expired', $2, $3, $4 FROM workspaces WHERE id = $1::uuid`, r.WorkspaceID, title, body, link); err != nil {
			return sent, err
		}
		sent++
	}
	return sent, nil
}

func reminderText(r reminder) (title, body, link string) {
	left := fmt.Sprintf("%d hari lagi", r.Days)
	if r.Days <= 1 {
		left = "besok atau kurang"
	}
	ws := oneLine(r.Workspace)
	switch r.Prefix {
	case "trial":
		return "Trial berakhir " + left, "Masa trial " + ws + " berakhir " + r.Ref.In(jakarta()).Format("2 Jan 2006") + ". Pilih paket dan unggah bukti transfer agar event WhatsApp tetap terkirim ke Meta.", "/langganan"
	case "token":
		return "Token Meta berakhir " + left, "Token Conversions API untuk " + ws + " berakhir " + r.Ref.In(jakarta()).Format("2 Jan 2006") + ". Buat token baru di Events Manager lalu simpan di halaman Koneksi sebelum itu.", "/koneksi"
	}
	return "Langganan berakhir " + left, "Langganan " + ws + " berakhir " + r.Ref.In(jakarta()).Format("2 Jan 2006") + ". Perpanjang agar event WhatsApp tetap terkirim ke Meta.", "/langganan"
}

func jakarta() *time.Location {
	if l, err := time.LoadLocation("Asia/Jakarta"); err == nil {
		return l
	}
	return time.FixedZone("WIB", 7*3600)
}

// ---------- Retensi & kebersihan ----------

// Retention menghapus data lama. Lama simpan log event dan webhook_inbox dibaca dari settings
// (retention_event_days, retention_inbox_days). Data keuangan (payments, payment_proofs) TIDAK dihapus otomatis.
func Retention(ctx context.Context, db *pgxpool.Pool) (map[string]int64, error) {
	out := map[string]int64{}
	var evDays, inboxDays int
	if err := db.QueryRow(ctx, `SELECT
	  GREATEST(60, COALESCE((SELECT (value #>> '{}')::int FROM settings WHERE key = 'retention_event_days'), 365)),
	  GREATEST(7,  COALESCE((SELECT (value #>> '{}')::int FROM settings WHERE key = 'retention_inbox_days'), 30))`).Scan(&evDays, &inboxDays); err != nil {
		return nil, err
	}
	steps := []struct {
		name, sql string
		args      []any
	}{
		{"events", `DELETE FROM events WHERE created_at < now() - make_interval(days => $1) AND status IN ('sent', 'failed', 'skipped')`, []any{evDays}},
		{"lead_messages", `DELETE FROM lead_messages WHERE sent_at < now() - interval '90 days'`, nil},
		{"webhook_inbox", `DELETE FROM webhook_inbox WHERE received_at < now() - make_interval(days => $1)`, []any{inboxDays}},
		{"sessions", `DELETE FROM sessions WHERE expires_at < now() - interval '7 days' OR revoked_at < now() - interval '7 days'`, nil},
		{"notifications", `DELETE FROM notifications WHERE read_at IS NOT NULL AND created_at < now() - interval '180 days'`, nil},
		{"reminder_log", `DELETE FROM reminder_log WHERE sent_at < now() - interval '1 year'`, nil},
		{"support_access_codes", `DELETE FROM support_access_codes WHERE expires_at < now() - interval '7 days'`, nil},
	}
	for _, s := range steps {
		tag, err := db.Exec(ctx, s.sql, s.args...)
		if err != nil {
			return out, fmt.Errorf("%s: %w", s.name, err)
		}
		out[s.name] = tag.RowsAffected()
	}
	return out, nil
}

// ---------- Penjadwal ----------

// Run menjalankan semua tugas: sekali saat mulai (dengan backfill rekap 35 hari), lalu berkala.
// Berhenti saat ctx dibatalkan.
// syncAds (boleh nil) dipanggil tiap 30 menit untuk menarik data iklan dari Meta.
func Run(ctx context.Context, db *pgxpool.Pool, syncAds func(context.Context)) {
	logErr := func(name string, err error) {
		if err != nil && ctx.Err() == nil {
			log.Printf("jobs/%s: %v", name, err)
		}
	}
	stats := func(back int) { logErr("stats", RecomputeStats(ctx, db, back, "")) }
	remind := func() {
		n, err := SendReminders(ctx, db)
		logErr("reminders", err)
		if n > 0 {
			log.Printf("jobs/reminders: %d pengingat dikirim", n)
		}
	}
	retain := func() {
		r, err := Retention(ctx, db)
		logErr("retention", err)
		var total int64
		for _, v := range r {
			total += v
		}
		if total > 0 {
			log.Printf("jobs/retention: %v", r)
		}
	}

	stats(35)
	remind()
	retain()
	if syncAds != nil {
		go syncAds(ctx)
	}

	statsT := time.NewTicker(5 * time.Minute)
	remindT := time.NewTicker(time.Hour)
	retainT := time.NewTicker(24 * time.Hour)
	adsT := time.NewTicker(30 * time.Minute)
	defer adsT.Stop()
	defer statsT.Stop()
	defer remindT.Stop()
	defer retainT.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-statsT.C:
			stats(2)
		case <-remindT.C:
			remind()
			stats(35) // data terlambat (mis. retry webhook) untuk hari yang lebih lama ikut terkoreksi
		case <-retainT.C:
			retain()
		case <-adsT.C:
			if syncAds != nil {
				syncAds(ctx)
			}
		}
	}
}
