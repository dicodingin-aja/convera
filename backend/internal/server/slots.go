package server

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/dicodingin-aja/convera/backend/internal/core"
	"github.com/gin-gonic/gin"
)

// ---------- Slot kampanye iklan (lihat internal/core/slots.go) ----------

// adSlots: GET /ad-slots. Kuota, pemakaian, target yang memakai slot, dan kampanye/iklan yang bisa dipilih.
func (s *Server) adSlots(c *gin.Context) {
	out, err := s.adSlotsState(c)
	if err != nil {
		serverError(c, err)
		return
	}
	ok(c, out)
}

func (s *Server) adSlotsState(c *gin.Context) (gin.H, error) {
	ws := wsID(c)
	quota, err := core.SlotQuota(c, s.DB, ws)
	if err != nil {
		return nil, err
	}
	used, err := core.SlotUsage(c, s.DB, ws)
	if err != nil {
		return nil, err
	}
	var connected bool
	_ = s.DB.QueryRow(c, `SELECT EXISTS (SELECT 1 FROM ad_connections WHERE workspace_id = $1)`, ws).Scan(&connected)
	var price float64
	_ = s.DB.QueryRow(c, `SELECT COALESCE((SELECT p.price_month::float8 FROM subscriptions sub JOIN plans p ON p.id = sub.plan_id WHERE sub.workspace_id = $1),
		(SELECT price_month::float8 FROM plans WHERE deleted_at IS NULL AND shown ORDER BY sort, created_at LIMIT 1), 0)`, ws).Scan(&price)

	// Target yang memakai slot. effective = sedang dilacak dan masuk kuota (yang lebih dulu dilacak didahulukan).
	tracked, err := query(c, s.DB, `
SELECT kind, target_id AS id, COALESCE(name, '') AS name, auto, started_at, stopped_at,
       started_at + make_interval(days => $3) AS locked_until,
       (stopped_at IS NULL AND row_number() OVER (ORDER BY started_at, id) <= $2) AS effective
FROM tracked_targets WHERE workspace_id = $1 AND (stopped_at IS NULL OR started_at + make_interval(days => $3) > now())
ORDER BY started_at, id`, ws, quota, core.SlotLockDays)
	if err != nil {
		return nil, err
	}

	// Pilihan: kampanye dari sinkron Meta Ads, lalu iklan dari chat yang kampanyenya belum diketahui (90 hari terakhir).
	// Chat tanpa ID iklan (organik) tidak muncul di sini.
	candidates, err := query(c, s.DB, `
WITH camp AS (
  SELECT 'campaign'::text AS kind, m.campaign_id AS id, max(m.campaign_name) AS name, count(DISTINCT m.ad_id)::bigint AS ads
  FROM ad_campaign_map m WHERE m.workspace_id = $1 GROUP BY m.campaign_id),
camp_stats AS (
  SELECT m.campaign_id AS id, count(DISTINCT l.id)::bigint AS leads_30d
  FROM ad_campaign_map m JOIN leads l ON l.workspace_id = m.workspace_id AND l.ad_id = m.ad_id
  WHERE m.workspace_id = $1 AND l.deleted_at IS NULL AND l.first_message_at > now() - interval '30 days' GROUP BY m.campaign_id),
camp_spend AS (
  SELECT campaign_id AS id, sum(spend)::float8 AS spend_30d FROM ad_insights_daily
  WHERE workspace_id = $1 AND campaign_id IS NOT NULL AND day > (now() AT TIME ZONE 'Asia/Jakarta')::date - 30 GROUP BY campaign_id),
ad AS (
  SELECT 'ad'::text AS kind, l.ad_id AS id, max(l.ad_headline) AS name, 1::bigint AS ads,
         count(*) FILTER (WHERE l.first_message_at > now() - interval '30 days')::bigint AS leads_30d
  FROM leads l WHERE l.workspace_id = $1 AND l.deleted_at IS NULL AND l.ad_id IS NOT NULL
    AND l.first_message_at > now() - interval '90 days'
    AND NOT EXISTS (SELECT 1 FROM ad_campaign_map m WHERE m.workspace_id = l.workspace_id AND m.ad_id = l.ad_id)
  GROUP BY l.ad_id)
SELECT x.kind, x.id, COALESCE(x.name, '') AS name, x.ads, x.leads_30d, x.spend_30d,
       EXISTS (SELECT 1 FROM tracked_targets t WHERE t.workspace_id = $1 AND t.kind = x.kind AND t.target_id = x.id AND t.stopped_at IS NULL) AS tracked
FROM (
  SELECT camp.kind, camp.id, camp.name, camp.ads, COALESCE(cs.leads_30d, 0) AS leads_30d, COALESCE(sp.spend_30d, 0) AS spend_30d
  FROM camp LEFT JOIN camp_stats cs ON cs.id = camp.id LEFT JOIN camp_spend sp ON sp.id = camp.id
  UNION ALL
  SELECT kind, id, name, ads, leads_30d, 0::float8 FROM ad) x
ORDER BY x.leads_30d DESC, x.spend_30d DESC, x.name`, ws)
	if err != nil {
		return nil, err
	}
	free := quota - used
	if free < 0 {
		free = 0
	}
	return gin.H{"quota": quota, "used": used, "free": free, "lock_days": core.SlotLockDays, "connected": connected,
		"price_per_slot": price, "tracked": tracked, "candidates": candidates}, nil
}

type slotTargetInput struct {
	Kind string `json:"kind"` // campaign | ad
	ID   string `json:"id"`
}

// trackAdSlot: POST /ad-slots/track {kind, id}. Memakai satu slot (kecuali target yang masih dalam masa kunci).
func (s *Server) trackAdSlot(c *gin.Context) {
	var in slotTargetInput
	if err := c.ShouldBindJSON(&in); err != nil || in.ID == "" {
		fail(c, http.StatusBadRequest, "bad_request", "Body tidak valid: kirim kind (campaign/ad) dan id.")
		return
	}
	err := core.TrackTarget(c, s.DB, wsID(c), in.Kind, in.ID)
	switch {
	case errors.Is(err, core.ErrSlotsFull):
		quota, _ := core.SlotQuota(c, s.DB, wsID(c))
		fail(c, http.StatusConflict, "slots_full", fmt.Sprintf(
			"Slot penuh (%d/%d). Tambah slot dulu untuk melacak kampanye ini.", quota, quota))
		return
	case errors.Is(err, core.ErrUnknownTarget):
		fail(c, http.StatusNotFound, "target_not_found", "Kampanye atau iklan tidak ditemukan di workspace ini.")
		return
	case err != nil:
		serverError(c, err)
		return
	}
	s.respondSlots(c)
}

// releaseAdSlot: POST /ad-slots/release {kind, id}. Berhenti melacak; slot tetap terpakai sampai masa kunci habis.
func (s *Server) releaseAdSlot(c *gin.Context) {
	var in slotTargetInput
	if err := c.ShouldBindJSON(&in); err != nil || in.ID == "" {
		fail(c, http.StatusBadRequest, "bad_request", "Body tidak valid: kirim kind (campaign/ad) dan id.")
		return
	}
	if err := core.ReleaseTarget(c, s.DB, wsID(c), in.Kind, in.ID); errors.Is(err, core.ErrTargetNotActive) {
		fail(c, http.StatusNotFound, "target_not_tracked", "Kampanye atau iklan ini tidak sedang dilacak.")
		return
	} else if err != nil {
		serverError(c, err)
		return
	}
	s.respondSlots(c)
}

func (s *Server) respondSlots(c *gin.Context) {
	out, err := s.adSlotsState(c)
	if err != nil {
		serverError(c, err)
		return
	}
	ok(c, out)
}
