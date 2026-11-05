package server

import (
	"crypto/sha256"
	"net/http"
	"strings"

	"github.com/dicodingin-aja/convera/backend/internal/core"
	"github.com/dicodingin-aja/convera/backend/internal/meta"
	"github.com/gin-gonic/gin"
)

// ---------- Event mapping (tabel event_rules) ----------

type ruleInput struct {
	Name       string   `json:"name"`
	Who        string   `json:"who"` // customer | admin | template
	Op         string   `json:"op"`  // contains | equals
	Keywords   []string `json:"keywords"`
	EventName  string   `json:"event_name"`
	ValueMode  string   `json:"value_mode"` // parse | fixed | none
	FixedValue *float64 `json:"fixed_value"`
	Active     *bool    `json:"active"`
}

func (s *Server) currentMode(c *gin.Context) string {
	mode := meta.ModeWABA
	_ = s.DB.QueryRow(c, `SELECT mode FROM meta_connections WHERE workspace_id=$1`, wsID(c)).Scan(&mode)
	return mode
}

func (s *Server) validateRule(c *gin.Context, in *ruleInput) bool {
	in.Name = strings.TrimSpace(in.Name)
	if in.Op == "" {
		in.Op = "contains"
	}
	if in.ValueMode == "" {
		in.ValueMode = "none"
	}
	var kws []string
	for _, k := range in.Keywords {
		if k = strings.TrimSpace(k); k != "" {
			kws = append(kws, k)
		}
	}
	in.Keywords = kws
	msg := ""
	switch {
	case in.Name == "" || len(in.Name) > 120:
		msg = "Nama aturan wajib diisi (maks. 120 karakter)."
	case in.Who != "customer" && in.Who != "admin" && in.Who != "template":
		msg = "who harus customer, admin, atau template."
	case in.Op != "contains" && in.Op != "equals":
		msg = "op harus contains atau equals."
	case len(in.Keywords) == 0:
		msg = "Isi minimal satu kata kunci."
	case in.ValueMode != "parse" && in.ValueMode != "fixed" && in.ValueMode != "none":
		msg = "value_mode harus parse, fixed, atau none."
	case in.ValueMode == "fixed" && (in.FixedValue == nil || *in.FixedValue < 0):
		msg = "fixed_value wajib diisi untuk value_mode fixed."
	default:
		if err := meta.ValidateEvent(s.currentMode(c), in.EventName, 1, "IDR"); err != nil {
			msg = err.Error()
		}
	}
	if msg != "" {
		fail(c, http.StatusUnprocessableEntity, "validation_error", msg)
		return false
	}
	return true
}

const ruleCols = `id::text AS id, name, who, op, keywords, event_name, value_mode, fixed_value::float8 AS fixed_value,
  active, sort, hit_count, last_hit_at, created_at`

func (s *Server) listRules(c *gin.Context) {
	rows, err := query(c, s.DB, `SELECT `+ruleCols+` FROM event_rules WHERE workspace_id=$1 AND deleted_at IS NULL ORDER BY sort, created_at`, wsID(c))
	if err != nil {
		serverError(c, err)
		return
	}
	ok(c, gin.H{"rules": rows, "allowed_events": meta.AllowedEvents(s.currentMode(c))})
}

func (s *Server) createRule(c *gin.Context) {
	var in ruleInput
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "bad_request", "Body tidak valid.")
		return
	}
	if !s.validateRule(c, &in) {
		return
	}
	active := in.Active == nil || *in.Active
	rows, err := query(c, s.DB, `
INSERT INTO event_rules (workspace_id, name, who, op, keywords, event_name, value_mode, fixed_value, active, sort)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9, COALESCE((SELECT max(sort)+1 FROM event_rules WHERE workspace_id=$1), 0))
RETURNING `+ruleCols, wsID(c), in.Name, in.Who, in.Op, in.Keywords, in.EventName, in.ValueMode, in.FixedValue, active)
	if err != nil {
		serverError(c, err)
		return
	}
	okStatus(c, http.StatusCreated, rows[0])
}

func (s *Server) updateRule(c *gin.Context) {
	var in ruleInput
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "bad_request", "Body tidak valid.")
		return
	}
	if !s.validateRule(c, &in) {
		return
	}
	active := in.Active == nil || *in.Active
	rows, err := query(c, s.DB, `
UPDATE event_rules SET name=$3, who=$4, op=$5, keywords=$6, event_name=$7, value_mode=$8, fixed_value=$9, active=$10
WHERE id::text=$1 AND workspace_id=$2 AND deleted_at IS NULL RETURNING `+ruleCols,
		c.Param("id"), wsID(c), in.Name, in.Who, in.Op, in.Keywords, in.EventName, in.ValueMode, in.FixedValue, active)
	if err != nil {
		serverError(c, err)
		return
	}
	if len(rows) == 0 {
		fail(c, http.StatusNotFound, "rule_not_found", "Aturan tidak ditemukan.")
		return
	}
	ok(c, rows[0])
}

func (s *Server) deleteRule(c *gin.Context) {
	tag, err := s.DB.Exec(c, `UPDATE event_rules SET deleted_at=now(), active=false WHERE id::text=$1 AND workspace_id=$2 AND deleted_at IS NULL`,
		c.Param("id"), wsID(c))
	if err != nil {
		serverError(c, err)
		return
	}
	if tag.RowsAffected() == 0 {
		fail(c, http.StatusNotFound, "rule_not_found", "Aturan tidak ditemukan.")
		return
	}
	ok(c, gin.H{"deleted": true})
}

// testRule: tempel contoh pesan → aturan mana yang cocok dan nilai yang terbaca. Tidak menyimpan apa pun.
func (s *Server) testRule(c *gin.Context) {
	var in struct {
		Who     string `json:"who"`
		Message string `json:"message"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.Message == "" {
		fail(c, http.StatusBadRequest, "bad_request", "message wajib diisi.")
		return
	}
	if in.Who == "" {
		in.Who = "customer"
	}
	rules, err := core.LoadRules(c, s.DB, wsID(c))
	if err != nil {
		serverError(c, err)
		return
	}
	matches := []gin.H{}
	for _, r := range rules {
		if !r.Matches(in.Who, in.Message) {
			continue
		}
		v, need := r.Value(in.Message)
		matches = append(matches, gin.H{"rule_id": r.ID, "name": r.Name, "event_name": r.EventName, "value": v, "needs_value": need || (r.EventName == "Purchase" && v == nil)})
	}
	ok(c, gin.H{"matched": len(matches) > 0, "matches": matches})
}

// ---------- API key per workspace ----------

func hashKey(key string) []byte {
	h := sha256.Sum256([]byte(key))
	return h[:]
}
