// Package core berisi logika bisnis yang dipakai bersama oleh webhook, API publik, dan dashboard:
// upsert lead, pembuatan event konversi, dan pencocokan aturan event.
// Isi pesan WhatsApp hanya lewat di memori; tidak ada fungsi di sini yang menyimpan atau me-log-nya.
package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dicodingin-aja/convera/backend/internal/meta"
	"github.com/dicodingin-aja/convera/backend/internal/wa"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// DBTX dipenuhi oleh *pgxpool.Pool maupun pgx.Tx.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// AttributionWindow adalah jendela atribusi mode WABA sejak klik iklan.
const AttributionWindow = 7 * 24 * time.Hour

var (
	ErrNoConnection = errors.New("koneksi Meta belum diatur untuk workspace ini")
	ErrLeadNotFound = errors.New("lead tidak ditemukan")
)

// ValidationError dipetakan ke HTTP 422.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

// ---------- Lead ----------

type LeadInput struct {
	WorkspaceID string
	Phone       string // sudah dinormalisasi 62xxxxxxxxxx
	Name        string
	PushName    string
	City        string
	CtwaClid    string
	AdID        string
	Headline    string
	SourceURL   string
	Source      string // webhook | api | qr | organic
	ClickedAt   time.Time
	MessageAt   time.Time
}

// UpsertLead membuat lead baru atau memperbarui yang ada (satu per nomor per workspace).
// Klik iklan baru (ctwa_clid ada) memulai jendela atribusi 7 hari yang baru.
func UpsertLead(ctx context.Context, db DBTX, in LeadInput) (string, error) {
	if in.MessageAt.IsZero() {
		in.MessageAt = time.Now().UTC()
	}
	source := in.Source
	var clicked, expires *time.Time
	if in.CtwaClid != "" {
		c := in.ClickedAt
		if c.IsZero() {
			c = in.MessageAt
		}
		e := c.Add(AttributionWindow)
		clicked, expires = &c, &e
	} else if source == "webhook" || source == "api" {
		source = "organic"
	}
	var id string
	err := db.QueryRow(ctx, `
INSERT INTO leads (workspace_id, phone, name, push_name, city, ctwa_clid, ad_id, ad_headline, source_url,
                   source, clicked_at, attribution_expires_at, first_message_at, last_message_at)
VALUES ($1,$2,NULLIF($3,''),NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),
        $10,$11,$12,$13,$13)
ON CONFLICT (workspace_id, phone) WHERE deleted_at IS NULL DO UPDATE SET
  name        = COALESCE(leads.name, EXCLUDED.name),
  push_name   = COALESCE(EXCLUDED.push_name, leads.push_name),
  city        = COALESCE(EXCLUDED.city, leads.city),
  ctwa_clid   = COALESCE(EXCLUDED.ctwa_clid, leads.ctwa_clid),
  ad_id       = CASE WHEN EXCLUDED.ctwa_clid IS NOT NULL THEN EXCLUDED.ad_id ELSE leads.ad_id END,
  ad_headline = CASE WHEN EXCLUDED.ctwa_clid IS NOT NULL THEN EXCLUDED.ad_headline ELSE leads.ad_headline END,
  source_url  = CASE WHEN EXCLUDED.ctwa_clid IS NOT NULL THEN EXCLUDED.source_url ELSE leads.source_url END,
  clicked_at  = CASE WHEN EXCLUDED.ctwa_clid IS NOT NULL THEN EXCLUDED.clicked_at ELSE leads.clicked_at END,
  attribution_expires_at = CASE WHEN EXCLUDED.ctwa_clid IS NOT NULL THEN EXCLUDED.attribution_expires_at
                                ELSE leads.attribution_expires_at END,
  source      = CASE WHEN EXCLUDED.ctwa_clid IS NOT NULL THEN EXCLUDED.source ELSE leads.source END,
  last_message_at = GREATEST(leads.last_message_at, EXCLUDED.last_message_at)
RETURNING id::text`,
		in.WorkspaceID, in.Phone, in.Name, in.PushName, in.City, in.CtwaClid, in.AdID, in.Headline, in.SourceURL,
		source, clicked, expires, in.MessageAt).Scan(&id)
	return id, err
}

// ---------- Event konversi ----------

type Params struct {
	WorkspaceID string
	LeadID      string
	EventName   string
	Value       *float64
	Currency    string // default IDR
	Source      string // rule | manual | api | test
	RuleID      string
	CreatedBy   string
	ExternalID  string
	EventTime   time.Time
	// NeedValue: nilai belum diketahui (aturan parse gagal). Event dibuat berstatus pending_value.
	NeedValue bool
}

type Outcome struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	Duplicate bool   `json:"duplicate"`
	Reason    string `json:"reason,omitempty"`
}

// CreateConversion memvalidasi dan menyimpan event konversi. Status awal:
//   - queued: siap dikirim worker
//   - pending_value: menunggu nilai diisi di dashboard
//   - failed (not_from_ad): mode WABA tanpa ctwa_clid
//   - skipped (attribution_expired): mode WABA di luar 7 hari sejak klik
//
// Event yang sama dua kali (event_id sama) tidak dibuat ulang: hasilnya Duplicate=true.
func CreateConversion(ctx context.Context, db DBTX, p Params) (*Outcome, error) {
	var mode string
	err := db.QueryRow(ctx, `SELECT mode FROM meta_connections WHERE workspace_id = $1`, p.WorkspaceID).Scan(&mode)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoConnection
	} else if err != nil {
		return nil, err
	}

	var ctwa *string
	var expires *time.Time
	err = db.QueryRow(ctx, `SELECT ctwa_clid, attribution_expires_at FROM leads
	                        WHERE id = $1 AND workspace_id = $2 AND deleted_at IS NULL`, p.LeadID, p.WorkspaceID).Scan(&ctwa, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrLeadNotFound
	} else if err != nil {
		return nil, err
	}

	cur := strings.ToUpper(p.Currency)
	if cur == "" {
		cur = "IDR"
	}
	val := 0.0
	if p.Value != nil {
		val = *p.Value
	}
	if p.EventName == "Purchase" && p.Value == nil {
		p.NeedValue = true
	}
	if p.NeedValue {
		// Hanya periksa nama event; nilai diisi nanti.
		if err := meta.ValidateEvent(mode, p.EventName, 1, cur); err != nil {
			return nil, &ValidationError{err.Error()}
		}
	} else if err := meta.ValidateEvent(mode, p.EventName, val, cur); err != nil {
		return nil, &ValidationError{err.Error()}
	}

	now := time.Now().UTC()
	evTime := p.EventTime
	if evTime.IsZero() {
		evTime = now
	}
	status, code, msg := "queued", "", ""
	switch {
	case mode == meta.ModeWABA && (ctwa == nil || *ctwa == ""):
		status, code, msg = "failed", "not_from_ad", "Bukan dari iklan: lead tidak punya ctwa_clid, tidak dikirim ke Meta."
	case mode == meta.ModeWABA && expires != nil && now.After(*expires):
		status, code, msg = "skipped", "attribution_expired", "Di luar jendela atribusi 7 hari sejak klik iklan, tidak dikirim."
	case p.NeedValue:
		status = "pending_value"
	}

	eventID := meta.EventID(p.WorkspaceID, p.LeadID, p.ExternalID, p.EventName, val, evTime)
	var id string
	err = db.QueryRow(ctx, `
INSERT INTO events (workspace_id, lead_id, rule_id, event_id, event_name, event_time, value, currency, source,
                    status, mode, error_code, error_message, created_by, next_retry_at)
VALUES ($1,$2,NULLIF($3,'')::uuid,$4,$5,$6,$7,$8,$9,$10,$11,NULLIF($12,''),NULLIF($13,''),NULLIF($14,'')::uuid,
        CASE WHEN $10 = 'queued' THEN now() ELSE NULL END)
ON CONFLICT (workspace_id, event_id) DO NOTHING
RETURNING id::text`,
		p.WorkspaceID, p.LeadID, p.RuleID, eventID, p.EventName, evTime, p.Value, cur, p.Source,
		status, mode, code, msg, p.CreatedBy).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		out := &Outcome{Duplicate: true, Status: "duplicate"}
		if e := db.QueryRow(ctx, `SELECT id::text FROM events WHERE workspace_id=$1 AND event_id=$2`, p.WorkspaceID, eventID).Scan(&out.ID); e != nil {
			return nil, e
		}
		return out, nil
	} else if err != nil {
		return nil, err
	}
	return &Outcome{ID: id, Status: status, Reason: msg}, nil
}

// ---------- Aturan event ----------

type Rule struct {
	ID         string
	Name       string
	Who        string // customer | admin | template
	Op         string // contains | equals
	Keywords   []string
	EventName  string
	ValueMode  string // parse | fixed | none
	FixedValue *float64
}

// Matches memeriksa satu aturan terhadap teks (tanpa memedulikan huruf besar/kecil).
func (r Rule) Matches(who, text string) bool {
	if r.Who != who {
		return false
	}
	t := strings.ToLower(strings.TrimSpace(text))
	for _, k := range r.Keywords {
		k = strings.ToLower(strings.TrimSpace(k))
		if k == "" {
			continue
		}
		if (r.Op == "equals" && t == k) || (r.Op != "equals" && strings.Contains(t, k)) {
			return true
		}
	}
	return false
}

// Value menghitung nilai event dari pesan. needValue=true bila aturan meminta parse tetapi nominal tidak terbaca.
func (r Rule) Value(text string) (v *float64, needValue bool) {
	switch r.ValueMode {
	case "fixed":
		return r.FixedValue, false
	case "parse":
		if n, ok := wa.ParseRupiah(text); ok {
			return &n, false
		}
		return nil, true
	}
	return nil, false
}

// LoadRules memuat aturan aktif workspace.
func LoadRules(ctx context.Context, db DBTX, workspaceID string) ([]Rule, error) {
	rows, err := db.Query(ctx, `
SELECT id::text, name, who, op, keywords, event_name, value_mode, fixed_value::float8
FROM event_rules WHERE workspace_id = $1 AND active AND deleted_at IS NULL ORDER BY sort, created_at`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Rule
	for rows.Next() {
		var r Rule
		if err := rows.Scan(&r.ID, &r.Name, &r.Who, &r.Op, &r.Keywords, &r.EventName, &r.ValueMode, &r.FixedValue); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ---------- Pemrosesan pesan ----------

// Message adalah pesan ternormalisasi dari sumber mana pun (Cloud API, forward, sesi QR).
// Text hanya dipakai di memori.
type Message struct {
	Provider   string // cloud_api | qr
	ID         string
	Direction  string // in | out
	Contact    string // nomor kontak (pelanggan), sudah 62xxxxxxxxxx
	Name       string
	PushName   string
	At         time.Time
	Text       string
	IsTemplate bool
	CtwaClid   string
	AdID       string
	Headline   string
	SourceURL  string
	LeadSource string // webhook | qr
}

// ProcessMessage dipanggil di dalam transaksi. Idempoten berdasarkan (provider, message_id).
// Mengembalikan error hanya untuk kegagalan tak terduga (agar transaksi di-rollback dan pengirim mengulang);
// kegagalan aturan individual dicatat ringkas di webhook_inbox.error tanpa isi pesan.
func ProcessMessage(ctx context.Context, tx DBTX, workspaceID string, m Message) (processed bool, err error) {
	tag, err := tx.Exec(ctx, `
INSERT INTO webhook_inbox (workspace_id, provider, message_id, direction) VALUES ($1,$2,$3,$4)
ON CONFLICT (provider, message_id) DO NOTHING`, workspaceID, m.Provider, m.ID, m.Direction)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		return false, nil // sudah pernah diproses
	}

	leadID, err := UpsertLead(ctx, tx, LeadInput{
		WorkspaceID: workspaceID, Phone: m.Contact, Name: m.Name, PushName: m.PushName,
		CtwaClid: m.CtwaClid, AdID: m.AdID, Headline: m.Headline, SourceURL: m.SourceURL,
		Source: m.LeadSource, MessageAt: m.At, ClickedAt: m.At,
	})
	if err != nil {
		return false, err
	}

	// Isi pesan disimpan agar pelanggan bisa melihat percakapan lead-nya (dihapus otomatis oleh job retensi).
	if body := truncRunes(strings.TrimSpace(m.Text), 4000); body != "" {
		if _, err := tx.Exec(ctx, `INSERT INTO lead_messages (workspace_id, lead_id, direction, body, sent_at) VALUES ($1,$2,$3,$4,$5)`,
			workspaceID, leadID, m.Direction, body, m.At); err != nil {
			return false, err
		}
	}

	who := "customer"
	if m.Direction == "out" {
		who = "admin"
		if m.IsTemplate {
			who = "template"
		}
	}
	rules, err := LoadRules(ctx, tx, workspaceID)
	if err != nil {
		return false, err
	}
	var problems []string
	for _, r := range rules {
		if !r.Matches(who, m.Text) {
			continue
		}
		val, need := r.Value(m.Text)
		out, cerr := CreateConversion(ctx, tx, Params{
			WorkspaceID: workspaceID, LeadID: leadID, EventName: r.EventName, Value: val,
			Source: "rule", RuleID: r.ID, NeedValue: need, EventTime: m.At,
		})
		var ve *ValidationError
		switch {
		case errors.As(cerr, &ve), errors.Is(cerr, ErrNoConnection):
			problems = append(problems, fmt.Sprintf("aturan %q: %v", r.Name, cerr))
			continue
		case cerr != nil:
			return false, cerr
		}
		if !out.Duplicate {
			if _, err := tx.Exec(ctx, `UPDATE event_rules SET hit_count = hit_count + 1, last_hit_at = now() WHERE id = $1`, r.ID); err != nil {
				return false, err
			}
		}
	}
	_, err = tx.Exec(ctx, `UPDATE webhook_inbox SET processed_at = now(), error = NULLIF($3,'')
	                       WHERE provider = $1 AND message_id = $2`, m.Provider, m.ID, strings.Join(problems, "; "))
	return true, err
}

func truncRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
