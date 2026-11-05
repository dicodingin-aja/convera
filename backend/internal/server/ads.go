package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/dicodingin-aja/convera/backend/internal/ads"
	"github.com/dicodingin-aja/convera/backend/internal/secure"
	"github.com/gin-gonic/gin"
)

// ---------- Koneksi Meta Ads (Marketing API, hanya baca) ----------

func (s *Server) adsConnection(c *gin.Context) (gin.H, error) {
	rows, err := query(c, s.DB, `SELECT ad_account_id, account_name, currency, status, last_error, last_sync_at, access_token_hint,
	  (SELECT count(*) FROM ad_insights_daily d WHERE d.workspace_id = ad_connections.workspace_id) AS rows
	  FROM ad_connections WHERE workspace_id = $1`, wsID(c))
	if err != nil {
		return nil, err
	}
	out := gin.H{"connected": false, "graph_configured": s.Cfg.GraphAPIVersion != ""}
	if len(rows) > 0 {
		for k, v := range rows[0] {
			out[k] = v
		}
		out["connected"] = true
	}
	return out, nil
}

func (s *Server) getAdsConnection(c *gin.Context) {
	out, err := s.adsConnection(c)
	if err != nil {
		serverError(c, err)
		return
	}
	ok(c, out)
}

// putAdsConnection: {ad_account_id, access_token}. Token diuji ke Meta (akun + izin baca Insights) sebelum disimpan
// terenkripsi; sinkronisasi pertama berjalan di latar belakang.
func (s *Server) putAdsConnection(c *gin.Context) {
	var in struct {
		AdAccountID string `json:"ad_account_id"`
		AccessToken string `json:"access_token"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "bad_request", "Body tidak valid.")
		return
	}
	if s.Cfg.GraphAPIVersion == "" {
		fail(c, http.StatusServiceUnavailable, "graph_version_missing", "GRAPH_API_VERSION belum diisi di server.")
		return
	}
	if !s.limiter("adsconn|"+wsID(c), 5.0/60, 5).Allow() {
		fail(c, http.StatusTooManyRequests, "rate_limited", "Terlalu sering mencoba. Coba lagi sebentar.")
		return
	}
	accID, err := ads.NormalizeAccountID(in.AdAccountID)
	if err != nil {
		bad422(c, err.Error())
		return
	}
	token := strings.TrimSpace(in.AccessToken)
	if len(token) < 20 || strings.ContainsAny(token, " \t\r\n") {
		bad422(c, "Access token tidak valid (terlalu pendek atau mengandung spasi).")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 40*time.Second)
	defer cancel()
	acc, err := s.Syncer.Client.Account(ctx, token, accID)
	if err == nil {
		y := time.Now().In(time.FixedZone("WIB", 7*3600)).AddDate(0, 0, -1).Format("2006-01-02")
		_, err = s.Syncer.Client.Insights(ctx, token, accID, y, y) // menguji izin ads_read ke Insights
	}
	if err != nil {
		var ae *ads.APIError
		if errors.As(err, &ae) {
			msg, _ := ae.Translate()
			bad422(c, msg)
		} else {
			fail(c, http.StatusBadGateway, "meta_unreachable", "Tidak bisa menghubungi Meta. Coba lagi.")
		}
		return
	}

	enc, err := s.Box.SealString(token)
	if err != nil {
		serverError(c, err)
		return
	}
	tx, err := s.DB.Begin(c)
	if err != nil {
		serverError(c, err)
		return
	}
	defer tx.Rollback(c)
	// Ganti akun iklan -> data lama dibuang agar tidak tercampur.
	if _, err := tx.Exec(c, `DELETE FROM ad_insights_daily WHERE workspace_id = $1 AND EXISTS
		(SELECT 1 FROM ad_connections WHERE workspace_id = $1 AND ad_account_id <> $2)`, wsID(c), accID); err != nil {
		serverError(c, err)
		return
	}
	if _, err := tx.Exec(c, `
INSERT INTO ad_connections (workspace_id, ad_account_id, account_name, currency, access_token_enc, access_token_hint, status)
VALUES ($1, $2, NULLIF($3,''), NULLIF($4,''), $5, $6, 'pending')
ON CONFLICT (workspace_id) DO UPDATE SET ad_account_id = EXCLUDED.ad_account_id, account_name = EXCLUDED.account_name, currency = EXCLUDED.currency,
  access_token_enc = EXCLUDED.access_token_enc, access_token_hint = EXCLUDED.access_token_hint, status = 'pending', last_error = NULL,
  last_sync_at = CASE WHEN ad_connections.ad_account_id = EXCLUDED.ad_account_id THEN ad_connections.last_sync_at END`,
		wsID(c), accID, acc.Name, acc.Currency, enc, secure.Hint(token)); err != nil {
		serverError(c, err)
		return
	}
	if err := tx.Commit(c); err != nil {
		serverError(c, err)
		return
	}
	s.syncInBackground(wsID(c), 0)
	out, err := s.adsConnection(c)
	if err != nil {
		serverError(c, err)
		return
	}
	ok(c, out)
}

// syncInBackground menjalankan sinkronisasi tanpa menahan request.
func (s *Server) syncInBackground(workspaceID string, daysBack int) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer cancel()
		_ = s.Syncer.SyncWorkspace(ctx, workspaceID, daysBack) // kegagalan tercatat di ad_connections.last_error
	}()
}

// syncAds: POST /connections/ads/sync[?full=1]. Minta sinkronisasi sekarang (full=1: 34 hari penuh).
func (s *Server) syncAds(c *gin.Context) {
	if !s.limiter("adssync|"+wsID(c), 1.0/120, 1).Allow() {
		fail(c, http.StatusTooManyRequests, "rate_limited", "Sinkronisasi baru saja dijalankan. Tunggu beberapa menit.")
		return
	}
	var n int
	_ = s.DB.QueryRow(c, `SELECT count(*) FROM ad_connections WHERE workspace_id = $1`, wsID(c)).Scan(&n)
	if n == 0 {
		fail(c, http.StatusNotFound, "no_connection", "Meta Ads belum tersambung.")
		return
	}
	days := 0
	if c.Query("full") == "1" {
		days = ads.FirstSyncDays
	}
	s.syncInBackground(wsID(c), days)
	okStatus(c, http.StatusAccepted, gin.H{"status": "syncing"})
}

func (s *Server) deleteAdsConnection(c *gin.Context) {
	tx, err := s.DB.Begin(c)
	if err != nil {
		serverError(c, err)
		return
	}
	defer tx.Rollback(c)
	for _, q := range []string{`DELETE FROM ad_insights_daily WHERE workspace_id = $1`, `DELETE FROM ad_connections WHERE workspace_id = $1`} {
		if _, err := tx.Exec(c, q, wsID(c)); err != nil {
			serverError(c, err)
			return
		}
	}
	if err := tx.Commit(c); err != nil {
		serverError(c, err)
		return
	}
	ok(c, gin.H{"connected": false})
}

// ---------- Performa: kampanye > ad set > iklan, biaya dari Meta + hasil dari Convera ----------

type perfNode struct {
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	Spend         float64     `json:"spend"`
	Impressions   int64       `json:"impressions"`
	Clicks        int64       `json:"clicks"`
	Conversations int64       `json:"conversations"`
	Leads         int64       `json:"leads"`
	Purchases     int64       `json:"purchases"`
	Value         float64     `json:"value"`
	Children      []*perfNode `json:"children,omitempty"`
}

func (n *perfNode) add(o *perfNode) {
	n.Spend += o.Spend
	n.Impressions += o.Impressions
	n.Clicks += o.Clicks
	n.Conversations += o.Conversations
	n.Leads += o.Leads
	n.Purchases += o.Purchases
	n.Value += o.Value
}

// performance: GET /performance?from&to. Biaya, klik, dan percakapan dari Meta; lead dan purchase dari Convera,
// digabung lewat ID iklan (referral.source_id pada pesan Click-to-WhatsApp).
func (s *Server) performance(c *gin.Context) {
	from, to, err := dateRange(c, 30)
	if err != nil {
		fail(c, http.StatusUnprocessableEntity, "validation_error", err.Error())
		return
	}
	f, t := from.Format("2006-01-02"), to.AddDate(0, 0, -1).Format("2006-01-02")
	conn, err := query(c, s.DB, `SELECT status, currency, account_name, last_error, last_sync_at FROM ad_connections WHERE workspace_id = $1`, wsID(c))
	if err != nil {
		serverError(c, err)
		return
	}
	if len(conn) == 0 {
		ok(c, gin.H{"connected": false})
		return
	}
	rows, err := query(c, s.DB, `
WITH ins AS (
  SELECT ad_id, max(ad_name) AS ad_name, max(adset_id) AS adset_id, max(adset_name) AS adset_name, max(campaign_id) AS campaign_id, max(campaign_name) AS campaign_name,
         sum(spend)::float8 AS spend, sum(impressions)::bigint AS impressions, sum(clicks)::bigint AS clicks, sum(conversations)::bigint AS conversations
  FROM ad_insights_daily WHERE workspace_id = $1 AND day BETWEEN $2::date AND $3::date GROUP BY ad_id),
ld AS (
  SELECT ad_id, count(*)::bigint AS leads, max(ad_headline) AS headline FROM leads
  WHERE workspace_id = $1 AND deleted_at IS NULL AND ad_id IS NOT NULL AND (first_message_at AT TIME ZONE 'Asia/Jakarta')::date BETWEEN $2::date AND $3::date GROUP BY ad_id),
pu AS (
  SELECT l.ad_id, count(*)::bigint AS purchases, COALESCE(sum(e.value), 0)::float8 AS value FROM events e JOIN leads l ON l.id = e.lead_id
  WHERE e.workspace_id = $1 AND e.event_name = 'Purchase' AND e.status = 'sent' AND e.source <> 'test' AND l.ad_id IS NOT NULL
    AND (e.event_time AT TIME ZONE 'Asia/Jakarta')::date BETWEEN $2::date AND $3::date GROUP BY l.ad_id),
ids AS (SELECT ad_id FROM ins UNION SELECT ad_id FROM ld UNION SELECT ad_id FROM pu)
SELECT ids.ad_id, COALESCE(ins.ad_name, ld.headline, 'Iklan ' || right(ids.ad_id, 6)) AS ad_name,
  COALESCE(ins.adset_id, '-') AS adset_id, COALESCE(ins.adset_name, 'Tanpa data biaya') AS adset_name,
  COALESCE(ins.campaign_id, '-') AS campaign_id, COALESCE(ins.campaign_name, 'Tanpa data biaya') AS campaign_name,
  COALESCE(ins.spend, 0) AS spend, COALESCE(ins.impressions, 0) AS impressions, COALESCE(ins.clicks, 0) AS clicks, COALESCE(ins.conversations, 0) AS conversations,
  COALESCE(ld.leads, 0) AS leads, COALESCE(pu.purchases, 0) AS purchases, COALESCE(pu.value, 0) AS value
FROM ids LEFT JOIN ins USING (ad_id) LEFT JOIN ld USING (ad_id) LEFT JOIN pu USING (ad_id) ORDER BY spend DESC, purchases DESC`, wsID(c), f, t)
	if err != nil {
		serverError(c, err)
		return
	}

	total := &perfNode{Name: "Total"}
	var campaigns []*perfNode
	cIdx, aIdx := map[string]*perfNode{}, map[string]*perfNode{}
	for _, r := range rows {
		leaf := &perfNode{ID: r["ad_id"].(string), Name: r["ad_name"].(string), Spend: r["spend"].(float64), Impressions: r["impressions"].(int64),
			Clicks: r["clicks"].(int64), Conversations: r["conversations"].(int64), Leads: r["leads"].(int64), Purchases: r["purchases"].(int64), Value: r["value"].(float64)}
		cid, aid := r["campaign_id"].(string), r["campaign_id"].(string)+"/"+r["adset_id"].(string)
		camp := cIdx[cid]
		if camp == nil {
			camp = &perfNode{ID: cid, Name: r["campaign_name"].(string)}
			cIdx[cid] = camp
			campaigns = append(campaigns, camp)
		}
		set := aIdx[aid]
		if set == nil {
			set = &perfNode{ID: r["adset_id"].(string), Name: r["adset_name"].(string)}
			aIdx[aid] = set
			camp.Children = append(camp.Children, set)
		}
		set.Children = append(set.Children, leaf)
		set.add(leaf)
		camp.add(leaf)
		total.add(leaf)
	}
	if campaigns == nil {
		campaigns = []*perfNode{}
	}
	out := gin.H{"connected": true, "totals": total, "campaigns": campaigns}
	for k, v := range conn[0] {
		out[k] = v
	}
	ok(c, out)
}
