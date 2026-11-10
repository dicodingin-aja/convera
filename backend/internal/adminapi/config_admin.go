package adminapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/dicodingin-aja/convera/backend/internal/httpx"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// ---------- Aturan langganan & bagi hasil (tabel settings) ----------

type ruleBounds struct {
	key      string
	min, max float64
}

var ruleKeys = []ruleBounds{
	{"trial_days", 1, 90}, {"pay_months_12", 1, 12}, {"grace_days", 0, 30}, {"komisi_pct", 0, 100}, {"kas_pct", 0, 100},
	{"retention_event_days", 60, 3650}, {"retention_inbox_days", 7, 365},
}

func (s *Server) loadRules(c *gin.Context, db httpx.DBTX) (map[string]float64, error) {
	rows, err := httpx.Query(c, db, `SELECT key, (value #>> '{}')::float8 AS v FROM settings WHERE key = ANY($1)`,
		[]string{"trial_days", "pay_months_12", "grace_days", "komisi_pct", "kas_pct", "retention_event_days", "retention_inbox_days"})
	if err != nil {
		return nil, err
	}
	out := map[string]float64{}
	for _, r := range rows {
		out[r["key"].(string)] = r["v"].(float64)
	}
	return out, nil
}

func (s *Server) getRules(c *gin.Context) {
	r, err := s.loadRules(c, s.DB)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	httpx.OK(c, r)
}

// putRules mengubah sebagian atau semua aturan. Perubahan komisi/kas hanya berlaku untuk pembayaran BERIKUTNYA;
// pembayaran lama menyimpan snapshot persentasenya.
func (s *Server) putRules(c *gin.Context) {
	var in map[string]*float64
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "bad_request", "Body tidak valid.")
		return
	}
	allowed := map[string]ruleBounds{}
	for _, b := range ruleKeys {
		allowed[b.key] = b
	}
	for k, v := range in {
		b, ok := allowed[k]
		if !ok {
			bad(c, "Pengaturan tidak dikenal: "+k)
			return
		}
		if v == nil || *v < b.min || *v > b.max {
			bad(c, k+" harus antara "+trimF(b.min)+" dan "+trimF(b.max)+".")
			return
		}
		if k != "komisi_pct" && k != "kas_pct" {
			if *v != float64(int(*v)) {
				bad(c, k+" harus bilangan bulat.")
				return
			}
		}
	}
	tx, err := s.DB.Begin(c)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	defer tx.Rollback(c)
	before, err := s.loadRules(c, tx)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	after := map[string]float64{}
	for k, v := range before {
		after[k] = v
	}
	for k, v := range in {
		after[k] = *v
	}
	if after["komisi_pct"]+after["kas_pct"] > 100 {
		bad(c, "komisi_pct + kas_pct tidak boleh lebih dari 100.")
		return
	}
	for k, v := range in {
		raw, _ := json.Marshal(*v)
		if _, err := tx.Exec(c, `INSERT INTO settings (key, value, updated_by) VALUES ($1, $2, $3::uuid)
			ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_by = EXCLUDED.updated_by`, k, raw, adminID(c)); err != nil {
			httpx.ServerError(c, err)
			return
		}
	}
	if err := s.audit(c, tx, "subscription_rules.update", "", "settings", "subscription-rules", map[string]any{"before": before, "after": after}); err != nil {
		httpx.ServerError(c, err)
		return
	}
	if err := tx.Commit(c); err != nil {
		httpx.ServerError(c, err)
		return
	}
	httpx.OK(c, after)
}

func trimF(f float64) string { return strings.TrimRight(strings.TrimRight(jsonNum(f), "0"), ".") }

func jsonNum(f float64) string { b, _ := json.Marshal(f); return string(b) }

// ---------- Feature flags ----------

func (s *Server) listFlags(c *gin.Context) {
	rows, err := httpx.Query(c, s.DB, `SELECT key, enabled, badge, description FROM feature_flags ORDER BY key`)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	httpx.OK(c, rows)
}

func (s *Server) putFlag(c *gin.Context) {
	var in struct {
		Enabled *bool   `json:"enabled"`
		Badge   *string `json:"badge"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || (in.Enabled == nil && in.Badge == nil) {
		httpx.Fail(c, http.StatusBadRequest, "bad_request", "Kirim enabled dan/atau badge.")
		return
	}
	if in.Badge != nil && len(*in.Badge) > 20 {
		bad(c, "badge maksimal 20 karakter.")
		return
	}
	s.withAudit(c, "feature_flag.update", "feature_flag", func(tx pgx.Tx) (string, map[string]any, error) {
		tag, err := tx.Exec(c, `UPDATE feature_flags SET enabled = COALESCE($2, enabled),
		  badge = CASE WHEN $3::text IS NULL THEN badge ELSE NULLIF($3, '') END WHERE key = $1`, c.Param("key"), in.Enabled, in.Badge)
		if err == nil && tag.RowsAffected() == 0 {
			err = pgx.ErrNoRows
		}
		return c.Param("key"), map[string]any{"enabled": in.Enabled, "badge": in.Badge}, err
	}, http.StatusOK)
}

// ---------- Maintenance ----------

func (s *Server) getMaintenance(c *gin.Context) {
	rows, err := httpx.Query(c, s.DB, `SELECT
	  COALESCE((SELECT (value #>> '{}')::boolean FROM settings WHERE key = 'maintenance'), false) AS enabled,
	  COALESCE((SELECT value #>> '{}' FROM settings WHERE key = 'maintenance_message'), '') AS message`)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	httpx.OK(c, rows[0])
}

func (s *Server) putMaintenance(c *gin.Context) {
	var in struct {
		Enabled *bool   `json:"enabled"`
		Message *string `json:"message"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || (in.Enabled == nil && in.Message == nil) {
		httpx.Fail(c, http.StatusBadRequest, "bad_request", "Kirim enabled dan/atau message.")
		return
	}
	if in.Message != nil && (len(strings.TrimSpace(*in.Message)) == 0 || len(*in.Message) > 300) {
		bad(c, "Pesan maintenance wajib diisi (maks. 300 karakter).")
		return
	}
	s.withAudit(c, "maintenance.update", "settings", func(tx pgx.Tx) (string, map[string]any, error) {
		if in.Enabled != nil {
			// Beritahu semua customer (lonceng) hanya saat status benar-benar berubah.
			var was bool
			_ = tx.QueryRow(c, `SELECT COALESCE((SELECT (value #>> '{}')::boolean FROM settings WHERE key = 'maintenance'), false)`).Scan(&was)
			if was != *in.Enabled {
				title, body := "Maintenance selesai", "Convera sudah kembali normal. Anda bisa membuat workspace dan event baru lagi."
				if *in.Enabled {
					title, body = "Convera sedang maintenance", "Selama maintenance Anda hanya bisa memantau data; membuat workspace atau event baru dinonaktifkan sementara."
					var m string
					if in.Message != nil {
						m = strings.TrimSpace(*in.Message)
					} else {
						_ = tx.QueryRow(c, `SELECT COALESCE((SELECT value #>> '{}' FROM settings WHERE key = 'maintenance_message'), '')`).Scan(&m)
					}
					if m != "" {
						body = m + " " + body
					}
				}
				if _, err := tx.Exec(c, `INSERT INTO notifications (user_id, type, title, body)
					SELECT id, 'maintenance', $1, $2 FROM users WHERE deleted_at IS NULL`, title, body); err != nil {
					return "", nil, err
				}
			}
			raw, _ := json.Marshal(*in.Enabled)
			if _, err := tx.Exec(c, `INSERT INTO settings (key, value, updated_by) VALUES ('maintenance', $1, $2::uuid)
				ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_by = EXCLUDED.updated_by`, raw, adminID(c)); err != nil {
				return "", nil, err
			}
		}
		if in.Message != nil {
			raw, _ := json.Marshal(strings.TrimSpace(*in.Message))
			if _, err := tx.Exec(c, `INSERT INTO settings (key, value, updated_by) VALUES ('maintenance_message', $1, $2::uuid)
				ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_by = EXCLUDED.updated_by`, raw, adminID(c)); err != nil {
				return "", nil, err
			}
		}
		return "maintenance", map[string]any{"enabled": in.Enabled}, nil
	}, http.StatusOK)
}

// ---------- Integrasi ----------

var integrationKeys = map[string]bool{"meta": true, "whatsapp": true, "notifyProof": true, "email": true, "storage": true, "sheets": true}

func (s *Server) getIntegrations(c *gin.Context) {
	rows, err := httpx.Query(c, s.DB, `SELECT COALESCE((SELECT value FROM settings WHERE key = 'integrations'), '{}'::jsonb) AS value`)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	httpx.OK(c, rows[0]["value"])
}

func (s *Server) putIntegrations(c *gin.Context) {
	var in map[string]bool
	if err := c.ShouldBindJSON(&in); err != nil || len(in) == 0 {
		httpx.Fail(c, http.StatusBadRequest, "bad_request", "Kirim objek {nama: true|false}.")
		return
	}
	for k := range in {
		if !integrationKeys[k] {
			bad(c, "Integrasi tidak dikenal: "+k)
			return
		}
	}
	patch, _ := json.Marshal(in)
	s.withAudit(c, "integrations.update", "settings", func(tx pgx.Tx) (string, map[string]any, error) {
		_, err := tx.Exec(c, `INSERT INTO settings (key, value, updated_by) VALUES ('integrations', $1::jsonb, $2::uuid)
			ON CONFLICT (key) DO UPDATE SET value = settings.value || EXCLUDED.value, updated_by = EXCLUDED.updated_by`, patch, adminID(c))
		return "integrations", map[string]any{"changes": in}, err
	}, http.StatusOK)
}
