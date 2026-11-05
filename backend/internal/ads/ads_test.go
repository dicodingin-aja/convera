package ads

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizeAccountID(t *testing.T) {
	for in, want := range map[string]string{"1234567890": "1234567890", "act_1234567890": "1234567890", " ACT_99999 ": "99999"} {
		if got, err := NormalizeAccountID(in); err != nil || got != want {
			t.Errorf("NormalizeAccountID(%q) = %q, %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "abc", "12", "act_", "123 456 789", "../123456", "1234567890123456789012345678"} {
		if _, err := NormalizeAccountID(bad); err == nil {
			t.Errorf("NormalizeAccountID(%q) seharusnya error", bad)
		}
	}
}

func TestTranslate(t *testing.T) {
	if m, inv := (&APIError{Code: 190, Message: "Invalid OAuth"}).Translate(); !inv || !strings.Contains(m, "token") {
		t.Errorf("190: %q %v", m, inv)
	}
	if m, inv := (&APIError{Code: 200, Message: "Permissions error"}).Translate(); inv || !strings.Contains(m, "ads_read") {
		t.Errorf("izin: %q %v", m, inv)
	}
	if m, _ := (&APIError{Code: 100, Message: "Unsupported get request. Object with ID does not exist"}).Translate(); !strings.Contains(m, "Ad Account ID salah") {
		t.Errorf("akun salah: %q", m)
	}
	if m, _ := (&APIError{Code: 17, Message: "limit"}).Translate(); !strings.Contains(m, "membatasi") {
		t.Errorf("rate limit: %q", m)
	}
}

// fakeGraph meniru Graph API: /act_ID dan /act_ID/insights dengan 2 halaman.
func fakeGraph(t *testing.T) (*httptest.Server, *[]string) {
	var seen []string
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path+"?"+r.URL.RawQuery)
		q := r.URL.Query()
		if q.Get("access_token") == "bad" {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"error":{"message":"Invalid OAuth access token.","code":190}}`))
			return
		}
		switch {
		case r.URL.Path == "/v99.0/act_555555":
			_, _ = w.Write([]byte(`{"name":"Kopi Senja Ads","currency":"IDR","account_status":1,"id":"act_555555"}`))
		case r.URL.Path == "/v99.0/act_555555/insights" && q.Get("page") == "2":
			_, _ = w.Write([]byte(`{"data":[{"date_start":"2026-11-02","ad_id":"A2","ad_name":"Iklan Dua","spend":"1000.5","impressions":"10","clicks":"1","account_currency":"IDR"}]}`))
		case r.URL.Path == "/v99.0/act_555555/insights":
			next := srv.URL + "/v99.0/act_555555/insights?page=2&access_token=tok"
			evil := "https://evil.example/steal?token=1"
			_ = evil
			_, _ = w.Write([]byte(`{"data":[{"date_start":"2026-11-01","ad_id":"A1","ad_name":"Iklan Satu","adset_id":"S1","adset_name":"Jaksel","campaign_id":"C1","campaign_name":"Promo",
			"spend":"150000.25","impressions":"1200","clicks":"88","account_currency":"IDR","actions":[{"action_type":"link_click","value":"80"},
			{"action_type":"onsite_conversion.messaging_conversation_started_7d","value":"12"}]},{"date_start":"","ad_id":"X"}],"paging":{"next":"` + next + `"}}`))
		default:
			w.WriteHeader(404)
		}
	}))
	return srv, &seen
}

func TestAccountAndInsights(t *testing.T) {
	srv, seen := fakeGraph(t)
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, Version: "v99.0"}
	ctx := context.Background()

	a, err := c.Account(ctx, "tok", "555555")
	if err != nil || a.Name != "Kopi Senja Ads" || a.Currency != "IDR" || a.Status != 1 {
		t.Fatalf("Account: %+v %v", a, err)
	}
	rows, err := c.Insights(ctx, "tok", "555555", "2026-11-01", "2026-11-05")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("harus 2 baris (halaman 1 dan 2; baris tanpa tanggal dibuang): %d", len(rows))
	}
	r := rows[0]
	if r.AdID != "A1" || r.Spend != 150000.25 || r.Impressions != 1200 || r.Clicks != 88 || r.Conversations != 12 || r.CampaignName != "Promo" || r.AdsetName != "Jaksel" {
		t.Errorf("baris 1: %+v", r)
	}
	if rows[1].Spend != 1000.5 || rows[1].Conversations != 0 {
		t.Errorf("baris 2: %+v", rows[1])
	}
	q := (*seen)[1]
	for _, want := range []string{"level=ad", "time_increment=1", "time_range=", "2026-11-01", "limit=500"} {
		if !strings.Contains(q, want) {
			t.Errorf("parameter %q hilang: %s", want, q)
		}
	}
	var tr map[string]string
	_ = json.Unmarshal([]byte(`{"since":"2026-11-01","until":"2026-11-05"}`), &tr)
}

func TestErrorsDoNotLeakToken(t *testing.T) {
	srv, _ := fakeGraph(t)
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, Version: "v99.0"}
	_, err := c.Account(context.Background(), "bad", "555555")
	ae, ok := err.(*APIError)
	if !ok || ae.Code != 190 {
		t.Fatalf("harus APIError 190: %v", err)
	}
	// jaringan mati: pesan error tidak boleh memuat URL/token
	c2 := &Client{BaseURL: "http://127.0.0.1:1", Version: "v99.0"}
	_, err = c2.Account(context.Background(), "SECRET-TOKEN", "555555")
	if err == nil || strings.Contains(err.Error(), "SECRET-TOKEN") || strings.Contains(err.Error(), "127.0.0.1") {
		t.Errorf("pesan error membocorkan token/URL: %v", err)
	}
	if _, err := (&Client{}).Account(context.Background(), "t", "555555"); err == nil {
		t.Error("tanpa versi Graph API harus error")
	}
}

func TestPagingOnlyFollowsSameHost(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write([]byte(`{"data":[{"date_start":"2026-11-01","ad_id":"A1","spend":"1"}],"paging":{"next":"https://evil.example/steal?access_token=x"}}`))
	}))
	defer srv.Close()
	rows, err := (&Client{BaseURL: srv.URL, Version: "v99.0"}).Insights(context.Background(), "tok", "555555", "2026-11-01", "2026-11-01")
	if err != nil || len(rows) != 1 || hits != 1 {
		t.Errorf("tautan ke host lain tidak boleh diikuti: rows=%d hits=%d err=%v", len(rows), hits, err)
	}
}
