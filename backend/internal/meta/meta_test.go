package meta

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestValidateEvent(t *testing.T) {
	if err := ValidateEvent(ModeWABA, "Purchase", 350000, "IDR"); err != nil {
		t.Error(err)
	}
	if ValidateEvent(ModeWABA, "Lead", 0, "") == nil {
		t.Error("Lead tidak boleh di mode WABA")
	}
	if ValidateEvent(ModeQR, "Lead", 0, "") != nil {
		t.Error("Lead boleh di mode QR")
	}
	if ValidateEvent(ModeQR, "Purchase", 0, "IDR") == nil {
		t.Error("Purchase tanpa value harus ditolak")
	}
	if ValidateEvent(ModeQR, "Purchase", 10, "") == nil {
		t.Error("Purchase tanpa currency harus ditolak")
	}
	if ValidateEvent("x", "Purchase", 10, "IDR") == nil {
		t.Error("mode tidak dikenal harus ditolak")
	}
}

func TestEventIDDedup(t *testing.T) {
	d := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)
	a := EventID("w1", "l1", "ORDER-1", "Purchase", 100, d)
	b := EventID("w1", "l2", "ORDER-1", "Purchase", 999, d.Add(48*time.Hour))
	if a != b {
		t.Error("external_id sama harus menghasilkan event_id sama")
	}
	c := EventID("w1", "l1", "", "Purchase", 100, d)
	e := EventID("w1", "l1", "", "Purchase", 100, d.Add(time.Hour))
	f := EventID("w1", "l1", "", "Purchase", 100, d.Add(24*time.Hour))
	if c != e || c == f {
		t.Error("dedup tanpa external_id harus per hari")
	}
	if !strings.HasPrefix(a, "cv_") {
		t.Error("prefix")
	}
}

func TestBuildEventWABA(t *testing.T) {
	v := 350000.0
	ev, err := BuildEvent(Input{Mode: ModeWABA, EventName: "Purchase", EventID: "cv_x", EventTime: time.Unix(1730778000, 0),
		Value: &v, Currency: "IDR", WABAID: "999", CtwaClid: "ARA"})
	if err != nil {
		t.Fatal(err)
	}
	if ev["action_source"] != "business_messaging" || ev["messaging_channel"] != "whatsapp" || ev["event_time"] != int64(1730778000) {
		t.Errorf("payload WABA salah: %v", ev)
	}
	ud := ev["user_data"].(map[string]any)
	if ud["ctwa_clid"] != "ARA" || ud["whatsapp_business_account_id"] != "999" {
		t.Errorf("user_data: %v", ud)
	}
	if _, err := BuildEvent(Input{Mode: ModeWABA, EventName: "Purchase"}); !errors.Is(err, ErrNotFromAd) {
		t.Error("tanpa ctwa_clid harus ErrNotFromAd")
	}
}

func TestBuildEventQR(t *testing.T) {
	v := 100.0
	ev, err := BuildEvent(Input{Mode: ModeQR, EventName: "Purchase", EventID: "cv_x", EventTime: time.Now(), Value: &v,
		Phone: "6281234567890", Name: "Rina Amelia", City: "Tangerang Selatan", LeadID: "lead-1"})
	if err != nil {
		t.Fatal(err)
	}
	if ev["action_source"] != "chat" {
		t.Errorf("action_source = %v, harus 'chat'", ev["action_source"])
	}
	raw, _ := json.Marshal(ev)
	for _, leak := range []string{"6281234567890", "Rina", "Tangerang"} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("data pribadi %q tidak ter-hash di payload", leak)
		}
	}
	ud := ev["user_data"].(map[string]any)
	for _, k := range []string{"ph", "fn", "ct", "country", "external_id"} {
		if _, ok := ud[k]; !ok {
			t.Errorf("user_data.%s hilang", k)
		}
	}
	min, _ := BuildEvent(Input{Mode: ModeQR, EventName: "Lead", Phone: "6281234567890"})
	if _, ok := min["user_data"].(map[string]any)["fn"]; ok {
		t.Error("field kosong tidak boleh dikirim")
	}
}

func TestClassify(t *testing.T) {
	if v := Classify(&Result{ErrCode: 190, ErrMessage: "Invalid OAuth"}); !v.InvalidAuth || v.Retry {
		t.Errorf("190: %+v", v)
	}
	if v := Classify(&Result{HTTPStatus: 503}); !v.Retry {
		t.Error("5xx harus retry")
	}
	if v := Classify(&Result{HTTPStatus: 400, ErrCode: 4}); !v.Retry {
		t.Error("rate limit harus retry")
	}
	if v := Classify(&Result{HTTPStatus: 400, ErrCode: 100, ErrMessage: "Invalid parameter: event_name"}); v.Retry || !strings.Contains(v.Message, "tidak didukung") {
		t.Errorf("event name: %+v", v)
	}
	if v := Classify(&Result{HTTPStatus: 400, ErrCode: 100, ErrSubcode: 33, ErrMessage: "Unsupported post request"}); v.Retry || !strings.Contains(v.Message, "Pixel/Dataset ID") {
		t.Errorf("dataset: %+v", v)
	}
	v := Classify(&Result{HTTPStatus: 400, ErrCode: 999, ErrMessage: "aneh", FBTraceID: "TRACE"})
	if v.Retry || !strings.Contains(v.Message, "aneh") || !strings.Contains(v.Message, "TRACE") {
		t.Errorf("lainnya: %+v", v)
	}
}

func TestRetryDelay(t *testing.T) {
	want := []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 6 * time.Hour}
	for i, w := range want {
		if d, ok := RetryDelay(i + 1); !ok || d != w {
			t.Errorf("attempt %d: %v %v", i+1, d, ok)
		}
	}
	if _, ok := RetryDelay(6); ok {
		t.Error("setelah 5 retry harus berhenti")
	}
}

func TestSend(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		if gotBody["access_token"] == "bad" {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"error":{"message":"Invalid OAuth","code":190,"fbtrace_id":"T1"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"events_received":1,"fbtrace_id":"T0"}`))
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, Version: "v99.0"}
	res, err := c.Send(context.Background(), "12345", "tok", "TEST1", []map[string]any{{"event_name": "Lead"}})
	if err != nil || !res.OK() || res.FBTraceID != "T0" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if gotPath != "/v99.0/12345/events" || gotBody["test_event_code"] != "TEST1" || gotBody["access_token"] != "tok" {
		t.Errorf("path=%s body=%v", gotPath, gotBody)
	}
	res, _ = c.Send(context.Background(), "12345", "bad", "", nil)
	if res.OK() || res.ErrCode != 190 || res.FBTraceID != "T1" {
		t.Errorf("error res: %+v", res)
	}
	if _, err := (&Client{}).Send(context.Background(), "1", "t", "", nil); err == nil {
		t.Error("tanpa versi Graph API harus error")
	}
}
