// Package dispatch mengambil event berstatus 'queued' dan mengirimkannya ke Meta Conversions API.
package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"time"

	"github.com/dicodingin-aja/convera/backend/internal/meta"
	"github.com/dicodingin-aja/convera/backend/internal/secure"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Dispatcher struct {
	DB   *pgxpool.Pool
	Box  *secure.Box
	Meta *meta.Client
	// Batch adalah jumlah event yang diambil per putaran (default 20).
	Batch int
	// Lease: berapa lama event "dipinjam" worker sebelum dianggap macet dan boleh diambil lagi.
	Lease time.Duration
}

// Run menjalankan putaran berkala sampai ctx dibatalkan.
func (d *Dispatcher) Run(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		n, err := d.RunOnce(ctx)
		if err != nil && ctx.Err() == nil {
			log.Printf("dispatch: %v", err)
		}
		if n > 0 {
			continue // masih ada antrean, langsung lanjut
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

type job struct {
	ID, WorkspaceID, EventID, EventName, Currency, Source string
	LeadID                                                *string
	Mode                                                  *string
	Value                                                 *float64
	EventTime                                             time.Time
	Attempts                                              int
}

// RunOnce mengambil satu batch (FOR UPDATE SKIP LOCKED) dan memprosesnya. Mengembalikan jumlah event diproses.
func (d *Dispatcher) RunOnce(ctx context.Context) (int, error) {
	batch, lease := d.Batch, d.Lease
	if batch <= 0 {
		batch = 20
	}
	if lease <= 0 {
		lease = 5 * time.Minute
	}
	// Pinjam event: geser next_retry_at agar worker lain tidak mengambilnya sementara kita mengirim.
	rows, err := d.DB.Query(ctx, `
UPDATE events SET next_retry_at = now() + make_interval(secs => $2)
WHERE id IN (SELECT id FROM events WHERE status = 'queued' AND next_retry_at <= now()
             ORDER BY next_retry_at LIMIT $1 FOR UPDATE SKIP LOCKED)
RETURNING id::text, workspace_id::text, lead_id::text, event_id, event_name, event_time, value::float8,
          currency, source, mode, attempts`, batch, lease.Seconds())
	if err != nil {
		return 0, err
	}
	var jobs []job
	for rows.Next() {
		var j job
		if err := rows.Scan(&j.ID, &j.WorkspaceID, &j.LeadID, &j.EventID, &j.EventName, &j.EventTime, &j.Value,
			&j.Currency, &j.Source, &j.Mode, &j.Attempts); err != nil {
			rows.Close()
			return 0, err
		}
		jobs = append(jobs, j)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, j := range jobs {
		if err := d.process(ctx, j); err != nil {
			log.Printf("dispatch event %s: %v", j.ID, err)
		}
	}
	return len(jobs), nil
}

func (d *Dispatcher) process(ctx context.Context, j job) error {
	var (
		connMode, status        string
		pixel, testCode, wabaID *string
		tokenEnc                []byte
		phone, name, city, ctwa *string
		leadID                  string
	)
	err := d.DB.QueryRow(ctx, `
SELECT c.mode, c.status, c.pixel_id, c.access_token_enc, c.test_event_code, c.waba_id
FROM meta_connections c WHERE c.workspace_id = $1`, j.WorkspaceID).
		Scan(&connMode, &status, &pixel, &tokenEnc, &testCode, &wabaID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (pixel == nil || len(tokenEnc) == 0)) {
		return d.fail(ctx, j.ID, "no_connection", "Koneksi Meta belum lengkap (Pixel/Dataset ID atau token kosong).")
	} else if err != nil {
		return err
	}
	mode := connMode
	if j.Mode != nil {
		mode = *j.Mode
	}
	if j.LeadID != nil {
		leadID = *j.LeadID
		err = d.DB.QueryRow(ctx, `SELECT phone, name, city, ctwa_clid FROM leads WHERE id = $1`, leadID).Scan(&phone, &name, &city, &ctwa)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
	}
	token, err := d.Box.OpenString(tokenEnc)
	if err != nil {
		return d.fail(ctx, j.ID, "decrypt_failed", "Token tersimpan tidak bisa dibaca. Simpan ulang token di halaman Koneksi.")
	}

	in := meta.Input{Mode: mode, EventName: j.EventName, EventID: j.EventID, EventTime: j.EventTime,
		Value: j.Value, Currency: j.Currency, LeadID: leadID}
	if wabaID != nil {
		in.WABAID = *wabaID
	}
	if ctwa != nil {
		in.CtwaClid = *ctwa
	}
	if phone != nil {
		in.Phone = *phone
	}
	if name != nil {
		in.Name = *name
	}
	if city != nil {
		in.City = *city
	}
	ev, err := meta.BuildEvent(in)
	if err != nil {
		code := "build_failed"
		if errors.Is(err, meta.ErrNotFromAd) {
			code = "not_from_ad"
		}
		return d.fail(ctx, j.ID, code, err.Error())
	}
	payload, _ := json.Marshal(ev) // tanpa token

	tc := ""
	if testCode != nil {
		tc = *testCode
	}
	res, sendErr := d.Meta.Send(ctx, *pixel, token, tc, []map[string]any{ev})
	if sendErr != nil { // jaringan: selalu boleh di-retry
		return d.retryOrFail(ctx, j, "network", sendErr.Error(), payload)
	}
	if res.OK() {
		return d.succeed(ctx, j, leadID, res, payload)
	}
	v := meta.Classify(res)
	if v.InvalidAuth {
		d.markInvalid(ctx, j.WorkspaceID, status, res)
	}
	code := "meta_" + itoa(res.ErrCode)
	if !v.Retry {
		_, err := d.DB.Exec(ctx, `UPDATE events SET status='failed', attempts=attempts+1, error_code=$2, error_message=$3,
			meta_response=$4, fbtrace_id=NULLIF($5,''), request_payload=$6, next_retry_at=NULL WHERE id=$1`,
			j.ID, code, v.Message, res.Body, res.FBTraceID, payload)
		return err
	}
	return d.retryOrFail(ctx, j, code, v.Message, payload)
}

func (d *Dispatcher) retryOrFail(ctx context.Context, j job, code, msg string, payload []byte) error {
	attempts := j.Attempts + 1
	delay, ok := meta.RetryDelay(attempts)
	if !ok {
		_, err := d.DB.Exec(ctx, `UPDATE events SET status='failed', attempts=$2, error_code=$3, error_message=$4,
			request_payload=$5, next_retry_at=NULL WHERE id=$1`, j.ID, attempts, code, msg, payload)
		return err
	}
	_, err := d.DB.Exec(ctx, `UPDATE events SET attempts=$2, error_code=$3, error_message=$4, request_payload=$5,
		next_retry_at = now() + make_interval(secs => $6) WHERE id=$1`, j.ID, attempts, code, msg, payload, delay.Seconds())
	return err
}

func (d *Dispatcher) fail(ctx context.Context, id, code, msg string) error {
	_, err := d.DB.Exec(ctx, `UPDATE events SET status='failed', error_code=$2, error_message=$3, next_retry_at=NULL WHERE id=$1`, id, code, msg)
	return err
}

func (d *Dispatcher) succeed(ctx context.Context, j job, leadID string, res *meta.Result, payload []byte) error {
	tx, err := d.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE events SET status='sent', sent_at=now(), attempts=attempts+1, error_code=NULL,
		error_message=NULL, meta_response=$2, fbtrace_id=NULLIF($3,''), request_payload=$4, next_retry_at=NULL WHERE id=$1`,
		j.ID, res.Body, res.FBTraceID, payload); err != nil {
		return err
	}
	if ls := LeadStatusFor(j.EventName); leadID != "" && ls != "" {
		// Status lead hanya maju (new < lead < checkout < purchase), tidak mundur.
		if _, err := tx.Exec(ctx, `
UPDATE leads SET status = $2, value = CASE WHEN $2 = 'purchase' THEN COALESCE($3::numeric, value) ELSE value END
WHERE id = $1 AND status <> 'lost' AND
  array_position(ARRAY['new','lead','checkout','purchase'], status) < array_position(ARRAY['new','lead','checkout','purchase'], $2)`,
			leadID, ls, j.Value); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE meta_connections SET status='connected', last_error_code=NULL, last_check_at=now()
		WHERE workspace_id=$1 AND status IN ('pending','error','invalid')`, j.WorkspaceID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// LeadStatusFor memetakan event ke status lead; string kosong = tidak mengubah status.
func LeadStatusFor(eventName string) string {
	switch eventName {
	case "Purchase":
		return "purchase"
	case "InitiateCheckout":
		return "checkout"
	case "LeadSubmitted", "Lead":
		return "lead"
	}
	return ""
}

// markInvalid menandai token tidak valid (kode 190) dan memberi tahu pemilik workspace sekali saja.
func (d *Dispatcher) markInvalid(ctx context.Context, workspaceID, prev string, res *meta.Result) {
	_, _ = d.DB.Exec(ctx, `UPDATE meta_connections SET status='invalid', last_error_code=$2, last_check_at=now() WHERE workspace_id=$1`,
		workspaceID, "190")
	if prev == "invalid" {
		return
	}
	_, _ = d.DB.Exec(ctx, `
INSERT INTO notifications (user_id, workspace_id, type, title, body, link)
SELECT w.owner_user_id, w.id, 'token_invalid', 'Token Meta tidak valid',
       'Event berhenti terkirim. Buat token baru di Events Manager lalu simpan di halaman Koneksi.', '/koneksi'
FROM workspaces w WHERE w.id = $1`, workspaceID)
}

func itoa(n int) string { return strconv.Itoa(n) }
