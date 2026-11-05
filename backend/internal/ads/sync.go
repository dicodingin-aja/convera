package ads

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/dicodingin-aja/convera/backend/internal/secure"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Syncer menyinkronkan data iklan ke database.
type Syncer struct {
	DB     *pgxpool.Pool
	Box    *secure.Box
	Client *Client
}

var wib = time.FixedZone("WIB", 7*3600)

// FirstSyncDays: jendela awal saat pertama kali tersambung; sinkronisasi berikutnya hanya 3 hari terakhir
// (Meta masih merevisi angka beberapa hari ke belakang).
const (
	FirstSyncDays = 34
	DeltaDays     = 3
)

// SyncWorkspace menarik Insights untuk satu workspace dan menggantikan data pada jendela tanggalnya.
// daysBack=0 -> otomatis (34 hari bila belum pernah sinkron, selain itu 3 hari).
func (s *Syncer) SyncWorkspace(ctx context.Context, workspaceID string, daysBack int) error {
	var accID string
	var tokEnc []byte
	var last *time.Time
	var prevStatus string
	err := s.DB.QueryRow(ctx, `SELECT ad_account_id, access_token_enc, last_sync_at, status FROM ad_connections WHERE workspace_id::text = $1`, workspaceID).
		Scan(&accID, &tokEnc, &last, &prevStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	} else if err != nil {
		return err
	}
	if daysBack <= 0 {
		daysBack = DeltaDays
		if last == nil {
			daysBack = FirstSyncDays
		}
	}
	token, err := s.Box.OpenString(tokEnc)
	if err != nil {
		return s.fail(ctx, workspaceID, prevStatus, "error", "Token tersimpan tidak bisa dibaca. Simpan ulang token.", false)
	}
	acc, err := s.Client.Account(ctx, token, accID)
	if err != nil {
		return s.handle(ctx, workspaceID, prevStatus, err)
	}
	// Tanggal mengikuti WIB (zona akun iklan umumnya Asia/Jakarta); Meta menafsirkan since/until di zona waktu akun.
	now := time.Now().In(wib)
	until := now.Format("2006-01-02")
	since := now.AddDate(0, 0, -daysBack).Format("2006-01-02")
	rows, err := s.Client.Insights(ctx, token, accID, since, until)
	if err != nil {
		return s.handle(ctx, workspaceID, prevStatus, err)
	}

	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// Jendela diganti utuh: baris yang sudah tidak ada di Meta ikut hilang, dan revisi angka masuk.
	if _, err := tx.Exec(ctx, `DELETE FROM ad_insights_daily WHERE workspace_id::text = $1 AND day >= $2::date`, workspaceID, since); err != nil {
		return err
	}
	for _, r := range rows {
		cur := r.Currency
		if cur == "" {
			cur = acc.Currency
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO ad_insights_daily (workspace_id, day, ad_id, campaign_id, campaign_name, adset_id, adset_name, ad_name, spend, impressions, clicks, conversations, currency)
VALUES ($1::uuid, $2::date, $3, NULLIF($4,''), NULLIF($5,''), NULLIF($6,''), NULLIF($7,''), NULLIF($8,''), $9, $10, $11, $12, NULLIF($13,''))
ON CONFLICT (workspace_id, day, ad_id) DO UPDATE SET campaign_id = EXCLUDED.campaign_id, campaign_name = EXCLUDED.campaign_name,
  adset_id = EXCLUDED.adset_id, adset_name = EXCLUDED.adset_name, ad_name = EXCLUDED.ad_name, spend = EXCLUDED.spend,
  impressions = EXCLUDED.impressions, clicks = EXCLUDED.clicks, conversations = EXCLUDED.conversations, currency = EXCLUDED.currency, synced_at = now()`,
			workspaceID, r.Day, r.AdID, r.CampaignID, r.CampaignName, r.AdsetID, r.AdsetName, r.AdName, r.Spend, r.Impressions, r.Clicks, r.Conversations, cur); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE ad_connections SET status = 'connected', last_error = NULL, last_sync_at = now(), account_name = $2, currency = NULLIF($3,'')
		WHERE workspace_id::text = $1`, workspaceID, acc.Name, acc.Currency); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// handle mencatat kegagalan sinkronisasi. Token tidak valid menandai koneksi 'invalid' dan memberi tahu pemilik sekali.
func (s *Syncer) handle(ctx context.Context, workspaceID, prevStatus string, err error) error {
	var ae *APIError
	if errors.As(err, &ae) {
		msg, invalid := ae.Translate()
		if invalid {
			return s.fail(ctx, workspaceID, prevStatus, "invalid", msg, true)
		}
		return s.fail(ctx, workspaceID, prevStatus, "error", msg, false)
	}
	return s.fail(ctx, workspaceID, prevStatus, "error", "Gagal menghubungi Meta: "+err.Error(), false)
}

func (s *Syncer) fail(ctx context.Context, workspaceID, prevStatus, status, msg string, notify bool) error {
	if _, err := s.DB.Exec(ctx, `UPDATE ad_connections SET status = $2, last_error = $3 WHERE workspace_id::text = $1`, workspaceID, status, msg); err != nil {
		return err
	}
	if notify && prevStatus != "invalid" {
		_, _ = s.DB.Exec(ctx, `INSERT INTO notifications (user_id, workspace_id, type, title, body, link)
			SELECT owner_user_id, id, 'ads_token_invalid', 'Koneksi Meta Ads terputus', $2, '/koneksi' FROM workspaces WHERE id::text = $1`, workspaceID, msg)
	}
	return fmt.Errorf("%s", msg)
}

// SyncAll menyinkronkan semua koneksi aktif (dipanggil berkala oleh worker). Koneksi 'invalid' dilewati sampai
// pengguna menyimpan token baru. Satu kegagalan tidak menghentikan yang lain.
func (s *Syncer) SyncAll(ctx context.Context) {
	rows, err := s.DB.Query(ctx, `SELECT c.workspace_id::text FROM ad_connections c JOIN workspaces w ON w.id = c.workspace_id
		WHERE w.deleted_at IS NULL AND c.status <> 'invalid' ORDER BY c.last_sync_at NULLS FIRST LIMIT 100`)
	if err != nil {
		log.Printf("ads sync: %v", err)
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		if err := s.SyncWorkspace(ctx, id, 0); err != nil {
			log.Printf("ads sync workspace %s: %v", id, err)
		}
		time.Sleep(500 * time.Millisecond) // ramah terhadap batas laju Meta
	}
}
