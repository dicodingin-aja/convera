// Package ads mengambil data iklan dari Meta Marketing API (Insights) dan menyimpannya per hari per iklan.
// Hanya membaca (izin ads_read); Convera tidak mengubah apa pun di akun iklan.
package ads

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ConversationAction adalah jenis "action" di Insights untuk percakapan WhatsApp/Messenger yang dimulai dari iklan.
// Nama ini dari dokumentasi Insights API dan perlu dicek pada akun nyata (lihat catatan di dokumentasi).
var ConversationActions = []string{"onsite_conversion.messaging_conversation_started_7d"}

const insightsFields = "ad_id,ad_name,adset_id,adset_name,campaign_id,campaign_name,spend,impressions,clicks,actions,account_currency"

var accountRe = regexp.MustCompile(`^[0-9]{5,25}$`)

// NormalizeAccountID menerima "123456789", "act_123456789", atau dengan spasi; mengembalikan hanya angkanya.
func NormalizeAccountID(s string) (string, error) {
	s = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s)), "act_")
	if !accountRe.MatchString(s) {
		return "", errors.New("Ad Account ID hanya berisi angka (mis. 1234567890 atau act_1234567890)")
	}
	return s, nil
}

// APIError adalah error dari Graph API.
type APIError struct {
	HTTPStatus int
	Code       int
	Subcode    int
	Message    string
}

func (e *APIError) Error() string { return fmt.Sprintf("meta error %d: %s", e.Code, e.Message) }

// Translate memberi pesan untuk pengguna dan menandai apakah token tidak valid (perlu disambungkan ulang).
func (e *APIError) Translate() (msg string, invalidToken bool) {
	m := strings.ToLower(e.Message)
	switch {
	case e.Code == 190:
		return "Access token tidak valid atau sudah dicabut. Buat token baru lalu simpan lagi.", true
	case e.Code == 10 || e.Code == 200 || strings.Contains(m, "permission"):
		return "Token tidak punya izin membaca iklan. Pastikan izin ads_read dipilih dan akun iklan ini diberikan ke pengguna/System User pemilik token.", false
	case e.Code == 100 && (strings.Contains(m, "does not exist") || strings.Contains(m, "unsupported get request") || e.Subcode == 33):
		return "Ad Account ID salah, atau token ini tidak punya akses ke akun iklan tersebut.", false
	case e.Code == 4 || e.Code == 17 || e.Code == 32 || e.Code == 613 || e.Code == 80000 || e.Code == 80003 || e.Code == 80004:
		return "Meta sedang membatasi permintaan. Coba lagi beberapa menit lagi.", false
	}
	return "Meta menolak permintaan: " + e.Message, false
}

// Client memanggil Graph API. Versi selalu dari konfigurasi.
type Client struct {
	BaseURL string
	Version string
	HTTP    *http.Client
}

func (c *Client) base() string {
	b := strings.TrimRight(c.BaseURL, "/")
	if b == "" {
		b = "https://graph.facebook.com"
	}
	return b
}

func (c *Client) httpc() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// get memanggil URL dan mengurai JSON. URL (yang memuat token) tidak pernah ikut di pesan error.
func (c *Client) get(ctx context.Context, rawURL string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, errors.New("permintaan tidak valid")
	}
	resp, err := c.httpc().Do(req)
	if err != nil {
		return nil, errors.New("gagal menghubungi Meta (jaringan)")
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	out := map[string]any{}
	_ = json.Unmarshal(body, &out)
	if e, ok := out["error"].(map[string]any); ok {
		ae := &APIError{HTTPStatus: resp.StatusCode, Code: intOf(e["code"]), Subcode: intOf(e["error_subcode"])}
		ae.Message, _ = e["message"].(string)
		if ae.Code == 0 {
			ae.Code = -1
		}
		return nil, ae
	}
	if resp.StatusCode >= 400 {
		return nil, &APIError{HTTPStatus: resp.StatusCode, Code: -1, Message: "HTTP " + strconv.Itoa(resp.StatusCode)}
	}
	return out, nil
}

func intOf(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case string:
		i, _ := strconv.Atoi(n)
		return i
	}
	return 0
}

func (c *Client) url(path string, q url.Values) string {
	return c.base() + "/" + c.Version + path + "?" + q.Encode()
}

// Account adalah info dasar akun iklan.
type Account struct {
	ID       string
	Name     string
	Currency string
	Status   int // 1 = aktif
}

// Account membaca nama, mata uang, dan status akun iklan; sekaligus menguji token.
func (c *Client) Account(ctx context.Context, token, accountID string) (*Account, error) {
	if c.Version == "" {
		return nil, errors.New("GRAPH_API_VERSION belum diisi di server")
	}
	q := url.Values{"fields": {"name,currency,account_status"}, "access_token": {token}}
	out, err := c.get(ctx, c.url("/act_"+accountID, q))
	if err != nil {
		return nil, err
	}
	a := &Account{ID: accountID, Status: intOf(out["account_status"])}
	a.Name, _ = out["name"].(string)
	a.Currency, _ = out["currency"].(string)
	return a, nil
}

// Row adalah satu baris Insights: satu iklan pada satu hari.
type Row struct {
	Day           string // YYYY-MM-DD (zona waktu akun iklan)
	AdID, AdName  string
	AdsetID       string
	AdsetName     string
	CampaignID    string
	CampaignName  string
	Spend         float64
	Impressions   int64
	Clicks        int64
	Conversations int
	Currency      string
}

const maxPages = 40

// Insights mengambil data per iklan per hari untuk rentang since..until (YYYY-MM-DD), mengikuti paginasi.
func (c *Client) Insights(ctx context.Context, token, accountID, since, until string) ([]Row, error) {
	if c.Version == "" {
		return nil, errors.New("GRAPH_API_VERSION belum diisi di server")
	}
	tr, _ := json.Marshal(map[string]string{"since": since, "until": until})
	q := url.Values{"level": {"ad"}, "fields": {insightsFields}, "time_range": {string(tr)}, "time_increment": {"1"}, "limit": {"500"}, "access_token": {token}}
	next := c.url("/act_"+accountID+"/insights", q)
	baseHost := ""
	if u, err := url.Parse(c.base()); err == nil {
		baseHost = u.Host
	}
	var rows []Row
	for page := 0; next != "" && page < maxPages; page++ {
		out, err := c.get(ctx, next)
		if err != nil {
			return nil, err
		}
		data, _ := out["data"].([]any)
		for _, d := range data {
			if m, ok := d.(map[string]any); ok {
				if r, ok := parseRow(m); ok {
					rows = append(rows, r)
				}
			}
		}
		next = ""
		if p, ok := out["paging"].(map[string]any); ok {
			if n, _ := p["next"].(string); n != "" {
				// Hanya ikuti tautan ke host Graph yang sama (token ikut di tautan itu).
				if u, err := url.Parse(n); err == nil && u.Host == baseHost {
					next = n
				}
			}
		}
	}
	return rows, nil
}

func parseRow(m map[string]any) (Row, bool) {
	str := func(k string) string { s, _ := m[k].(string); return s }
	r := Row{Day: str("date_start"), AdID: str("ad_id"), AdName: str("ad_name"), AdsetID: str("adset_id"), AdsetName: str("adset_name"),
		CampaignID: str("campaign_id"), CampaignName: str("campaign_name"), Currency: str("account_currency")}
	if r.Day == "" || r.AdID == "" {
		return r, false
	}
	r.Spend, _ = strconv.ParseFloat(str("spend"), 64)
	r.Impressions, _ = strconv.ParseInt(str("impressions"), 10, 64)
	r.Clicks, _ = strconv.ParseInt(str("clicks"), 10, 64)
	if acts, ok := m["actions"].([]any); ok {
		for _, a := range acts {
			am, _ := a.(map[string]any)
			t, _ := am["action_type"].(string)
			for _, want := range ConversationActions {
				if t == want {
					v, _ := strconv.ParseFloat(fmt.Sprint(am["value"]), 64)
					r.Conversations += int(v)
				}
			}
		}
	}
	return r, true
}
