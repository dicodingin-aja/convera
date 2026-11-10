package core

// Slot kampanye iklan.
//
// Customer membayar per kampanye Meta yang dilacak (harga paket per bulan x jumlah slot). Satu slot = satu kampanye.
// Bila kampanye sebuah iklan belum diketahui (akun Meta Ads belum tersambung/tersinkron), satu iklan = satu slot;
// setelah sinkron, target iklan digabung ke kampanyenya (ConsolidateTracked).
//
// Aturan:
//   - Kuota = slot langganan yang sedang berjalan (atau periode terakhir selama masa tenggang, atau trial) + slot
//     tambahan yang masih berlaku. Workspace tanpa akses langganan: kuota 0.
//   - Target yang mulai dilacak memakai slot selama SlotLockDays sejak mulai, walau dilepas lebih awal.
//   - Bila kuota turun di bawah pemakaian, target yang lebih dulu dilacak yang tetap berlaku.
//   - Slot kosong diisi otomatis oleh kampanye pertama yang mendatangkan event (auto = true).
//   - Chat tanpa ID iklan (organik) tidak memakai slot dan tetap diproses.

import (
	"context"
	"errors"
	"strings"

	"github.com/dicodingin-aja/convera/backend/internal/billing"
	"github.com/jackc/pgx/v5"
)

// SlotLockDays: lama satu target memakai slot sejak mulai dilacak.
const SlotLockDays = 30

// countedSQL: baris tracked_targets yang memakai slot sekarang.
const countedSQL = `(stopped_at IS NULL OR started_at + make_interval(days => 30) > now())`

var (
	ErrSlotsFull       = errors.New("slot kampanye penuh")
	ErrUnknownTarget   = errors.New("kampanye atau iklan tidak dikenal")
	ErrTargetNotActive = errors.New("kampanye atau iklan ini tidak sedang dilacak")
)

// Beginner dipenuhi oleh *pgxpool.Pool.
type Beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// SlotQuota mengembalikan jumlah slot yang berlaku sekarang.
func SlotQuota(ctx context.Context, db DBTX, workspaceID string) (int, error) {
	ok, err := WorkspaceAccess(ctx, db, workspaceID)
	if err != nil || !ok {
		return 0, err
	}
	var base, extra *int
	err = db.QueryRow(ctx, `SELECT
  (SELECT ad_slots FROM payments WHERE workspace_id::text = $1 AND purchase = 'subscription'
     AND period_start <= now() AND period_end + `+billing.GraceSQL+` > now()
   ORDER BY (period_end > now()) DESC, period_end DESC LIMIT 1),
  (SELECT sum(ad_slots)::int FROM payments WHERE workspace_id::text = $1 AND purchase = 'slots'
     AND period_start <= now() AND period_end > now())`, workspaceID).Scan(&base, &extra)
	if err != nil {
		return 0, err
	}
	n := 0
	if base != nil {
		n = *base
	} else {
		// Trial (atau langganan aktif tanpa catatan pembayaran): slot trial dari pengaturan, default 1.
		var trial int
		if err := db.QueryRow(ctx, `SELECT COALESCE((SELECT (value #>> '{}')::int FROM settings WHERE key = 'trial_ad_slots'), 1)`).Scan(&trial); err != nil {
			return 0, err
		}
		n = trial
	}
	if extra != nil {
		n += *extra
	}
	return n, nil
}

// SlotUsage: jumlah slot yang terpakai sekarang (termasuk target yang dilepas tapi masih dalam masa kunci).
func SlotUsage(ctx context.Context, db DBTX, workspaceID string) (int, error) {
	var n int
	err := db.QueryRow(ctx, `SELECT count(*) FROM tracked_targets WHERE workspace_id::text = $1 AND `+countedSQL, workspaceID).Scan(&n)
	return n, err
}

// Target adalah sesuatu yang memakai slot: kampanye, atau iklan bila kampanyenya belum diketahui.
type Target struct {
	Kind string // campaign | ad
	ID   string
	Name string
}

// TargetsForAd: kampanye iklan ini (bila sudah diketahui dari sinkron Meta Ads) lalu iklan itu sendiri.
// Elemen pertama adalah target yang dipakai saat mengisi slot.
func TargetsForAd(ctx context.Context, db DBTX, workspaceID, adID string) ([]Target, error) {
	var out []Target
	var cid string
	var cname *string
	err := db.QueryRow(ctx, `SELECT campaign_id, campaign_name FROM ad_campaign_map WHERE workspace_id::text = $1 AND ad_id = $2`, workspaceID, adID).Scan(&cid, &cname)
	if err == nil {
		out = append(out, Target{Kind: "campaign", ID: cid, Name: deref(cname)})
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	var aname *string
	_ = db.QueryRow(ctx, `SELECT COALESCE((SELECT ad_name FROM ad_campaign_map WHERE workspace_id::text = $1 AND ad_id = $2),
		(SELECT ad_headline FROM leads WHERE workspace_id::text = $1 AND ad_id = $2 AND ad_headline IS NOT NULL ORDER BY first_message_at DESC LIMIT 1))`,
		workspaceID, adID).Scan(&aname)
	out = append(out, Target{Kind: "ad", ID: adID, Name: deref(aname)})
	return out, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// effective: target sedang dilacak dan termasuk dalam kuota (yang lebih dulu dilacak didahulukan).
func effective(ctx context.Context, db DBTX, workspaceID string, t Target, quota int) (bool, error) {
	var ok bool
	err := db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM (
  SELECT kind, target_id, stopped_at, row_number() OVER (ORDER BY started_at, id) AS rn
  FROM tracked_targets WHERE workspace_id::text = $1 AND `+countedSQL+`) x
WHERE x.kind = $2 AND x.target_id = $3 AND x.stopped_at IS NULL AND x.rn <= $4)`, workspaceID, t.Kind, t.ID, quota).Scan(&ok)
	return ok, err
}

// lockSlots mengunci baris langganan workspace agar perubahan slot dari beberapa worker/request berurutan.
func lockSlots(ctx context.Context, tx pgx.Tx, workspaceID string) error {
	_, err := tx.Exec(ctx, `SELECT 1 FROM subscriptions WHERE workspace_id::text = $1 FOR UPDATE`, workspaceID)
	return err
}

// EnsureAdTracked memutuskan apakah event dari iklan adID boleh dikirim ke Meta. Bila kampanye/iklannya belum
// dilacak dan masih ada slot kosong, slot diisi otomatis. Mengembalikan false bila slot penuh.
func EnsureAdTracked(ctx context.Context, db Beginner, workspaceID, adID string) (bool, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	if err := lockSlots(ctx, tx, workspaceID); err != nil {
		return false, err
	}
	quota, err := SlotQuota(ctx, tx, workspaceID)
	if err != nil {
		return false, err
	}
	targets, err := TargetsForAd(ctx, tx, workspaceID, adID)
	if err != nil {
		return false, err
	}
	for _, t := range targets {
		if ok, err := effective(ctx, tx, workspaceID, t, quota); err != nil || ok {
			return ok, err
		}
	}
	primary := targets[0]
	changed, err := track(ctx, tx, workspaceID, primary, true, quota)
	if errors.Is(err, ErrSlotsFull) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	if !changed {
		return false, nil // sudah dilacak, tetapi di luar kuota
	}
	// Target yang diaktifkan lagi dari masa kunci bisa saja tetap di luar kuota; cek ulang.
	ok, err := effective(ctx, tx, workspaceID, primary, quota)
	if err != nil {
		return false, err
	}
	return ok, tx.Commit(ctx)
}

// track mulai melacak target di dalam tx yang sudah mengunci slot. Target yang dilepas tapi masih dalam masa
// kunci diaktifkan lagi tanpa slot baru. Mengembalikan false bila target sudah aktif.
func track(ctx context.Context, tx pgx.Tx, workspaceID string, t Target, auto bool, quota int) (bool, error) {
	var active bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM tracked_targets WHERE workspace_id::text = $1 AND kind = $2 AND target_id = $3 AND stopped_at IS NULL)`,
		workspaceID, t.Kind, t.ID).Scan(&active); err != nil {
		return false, err
	}
	if active {
		return false, nil
	}
	tag, err := tx.Exec(ctx, `UPDATE tracked_targets SET stopped_at = NULL WHERE id = (
  SELECT id FROM tracked_targets WHERE workspace_id::text = $1 AND kind = $2 AND target_id = $3 AND stopped_at IS NOT NULL
    AND started_at + make_interval(days => 30) > now() ORDER BY started_at DESC LIMIT 1)`, workspaceID, t.Kind, t.ID)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() > 0 {
		return true, nil
	}
	used, err := SlotUsage(ctx, tx, workspaceID)
	if err != nil {
		return false, err
	}
	if used >= quota {
		return false, ErrSlotsFull
	}
	_, err = tx.Exec(ctx, `INSERT INTO tracked_targets (workspace_id, kind, target_id, name, auto) VALUES ($1::uuid, $2, $3, NULLIF($4,''), $5)`,
		workspaceID, t.Kind, t.ID, t.Name, auto)
	return err == nil, err
}

// TrackTarget: customer memilih kampanye/iklan untuk dilacak. Target harus dikenal (kampanye dari sinkron Meta Ads,
// atau iklan yang pernah mendatangkan lead).
func TrackTarget(ctx context.Context, db Beginner, workspaceID, kind, id string) error {
	id = strings.TrimSpace(id)
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := lockSlots(ctx, tx, workspaceID); err != nil {
		return err
	}
	var name *string
	switch kind {
	case "campaign":
		err = tx.QueryRow(ctx, `SELECT max(campaign_name) FROM ad_campaign_map WHERE workspace_id::text = $1 AND campaign_id = $2 HAVING count(*) > 0`, workspaceID, id).Scan(&name)
	case "ad":
		err = tx.QueryRow(ctx, `SELECT max(ad_headline) FROM leads WHERE workspace_id::text = $1 AND ad_id = $2 HAVING count(*) > 0`, workspaceID, id).Scan(&name)
	default:
		return ErrUnknownTarget
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrUnknownTarget
	} else if err != nil {
		return err
	}
	quota, err := SlotQuota(ctx, tx, workspaceID)
	if err != nil {
		return err
	}
	if _, err := track(ctx, tx, workspaceID, Target{Kind: kind, ID: id, Name: deref(name)}, false, quota); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ReleaseTarget berhenti melacak. Slotnya tetap terpakai sampai SlotLockDays sejak mulai dilacak.
func ReleaseTarget(ctx context.Context, db DBTX, workspaceID, kind, id string) error {
	tag, err := db.Exec(ctx, `UPDATE tracked_targets SET stopped_at = now() WHERE workspace_id::text = $1 AND kind = $2 AND target_id = $3 AND stopped_at IS NULL`,
		workspaceID, kind, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrTargetNotActive
	}
	return nil
}

// ConsolidateTracked menggabungkan target iklan ke kampanyenya setelah peta iklan -> kampanye diketahui.
// Target iklan yang kampanyenya sudah dilacak dihapus (tanpa masa kunci, karena ini tindakan sistem); sisanya
// diubah menjadi target kampanye dengan waktu mulai yang sama.
func ConsolidateTracked(ctx context.Context, db Beginner, workspaceID string) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := lockSlots(ctx, tx, workspaceID); err != nil {
		return err
	}
	// Hapus target iklan yang berlebih: kampanyenya sudah punya target dengan status aktif yang sama, atau ada target
	// iklan lain dari kampanye yang sama yang dimulai lebih dulu.
	if _, err := tx.Exec(ctx, `
WITH m AS (
  SELECT t.id, mp.campaign_id, t.stopped_at IS NULL AS active,
         row_number() OVER (PARTITION BY mp.campaign_id, t.stopped_at IS NULL ORDER BY t.started_at, t.id) AS rn
  FROM tracked_targets t JOIN ad_campaign_map mp ON mp.workspace_id = t.workspace_id AND mp.ad_id = t.target_id
  WHERE t.workspace_id::text = $1 AND t.kind = 'ad')
DELETE FROM tracked_targets x USING m WHERE x.id = m.id AND (m.rn > 1 OR EXISTS (
  SELECT 1 FROM tracked_targets c WHERE c.workspace_id = x.workspace_id AND c.kind = 'campaign' AND c.target_id = m.campaign_id
    AND (c.stopped_at IS NULL) = m.active))`, workspaceID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
UPDATE tracked_targets t SET kind = 'campaign', target_id = mp.campaign_id, name = COALESCE(mp.campaign_name, t.name)
FROM ad_campaign_map mp WHERE mp.workspace_id = t.workspace_id AND mp.ad_id = t.target_id AND t.workspace_id::text = $1 AND t.kind = 'ad'`, workspaceID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
