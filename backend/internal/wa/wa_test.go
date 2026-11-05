package wa

import "testing"

func TestNormalizePhone(t *testing.T) {
	ok := map[string]string{
		"081234567890":      "6281234567890",
		"+62 812-3456-7890": "6281234567890",
		"6281234567890":     "6281234567890",
		"81234567890":       "6281234567890",
	}
	for in, want := range ok {
		got, err := NormalizePhone(in)
		if err != nil || got != want {
			t.Errorf("NormalizePhone(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "abc", "0812", "12345678901", "+1 415 555 0100"} {
		if _, err := NormalizePhone(bad); err == nil {
			t.Errorf("NormalizePhone(%q) seharusnya error", bad)
		}
	}
}

func TestMaskPhone(t *testing.T) {
	if got := MaskPhone("6281234567890"); got != "6281****7890" {
		t.Errorf("MaskPhone = %q", got)
	}
}

func TestParseRupiah(t *testing.T) {
	cases := []struct {
		in   string
		want float64
		ok   bool
	}{
		{"Pembayaran diterima Rp 350.000, terima kasih", 350000, true},
		{"Rp350.000", 350000, true},
		{"Rp. 1.250.000", 1250000, true},
		{"total 350rb ya", 350000, true},
		{"350k", 350000, true},
		{"1,5jt", 1500000, true},
		{"2 juta", 2000000, true},
		{"Rp 450000", 450000, true},
		{"booking untuk 2 orang", 0, false},
		{"pembayaran diterima", 0, false},
		{"Rp 0", 0, false},
	}
	for _, c := range cases {
		got, ok := ParseRupiah(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("ParseRupiah(%q) = %v,%v; want %v,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestVerifySignature(t *testing.T) {
	body := []byte(`{"a":1}`)
	sig := Sign(body, "secret")
	if !VerifySignature(body, sig, "secret") {
		t.Error("signature valid ditolak")
	}
	if VerifySignature(body, sig, "other") {
		t.Error("secret salah diterima")
	}
	if VerifySignature([]byte(`{"a":2}`), sig, "secret") {
		t.Error("body berubah diterima")
	}
	for _, h := range []string{"", "sha256=", "sha256=zz", "md5=abc"} {
		if VerifySignature(body, h, "secret") {
			t.Errorf("header %q diterima", h)
		}
	}
	if VerifySignature(body, sig, "") {
		t.Error("secret kosong diterima")
	}
}

func TestParseCloudWebhook(t *testing.T) {
	body := []byte(`{"entry":[{"changes":[{"value":{
	  "metadata":{"phone_number_id":"111"},
	  "contacts":[{"wa_id":"6281234567890","profile":{"name":"Rina"}}],
	  "messages":[
	    {"from":"6281234567890","id":"wamid.A","timestamp":"1730778000","type":"text","text":{"body":"Halo"},
	     "referral":{"source_type":"ad","source_id":"1202","headline":"Promo","ctwa_clid":"ARA"}},
	    {"from":"6281234567891","id":"wamid.B","timestamp":"1730778001","type":"text","text":{"body":"organik"}},
	    {"from":"","id":"","type":"text"}
	  ]}}]}]}`)
	msgs, err := ParseCloudWebhook(body)
	if err != nil || len(msgs) != 2 {
		t.Fatalf("err=%v len=%d", err, len(msgs))
	}
	if !msgs[0].Referral.IsAd() || msgs[0].Referral.CtwaClid != "ARA" || msgs[0].Name != "Rina" || msgs[0].PhoneNumberID != "111" {
		t.Errorf("pesan 0 salah: %+v", msgs[0])
	}
	if msgs[1].Referral.IsAd() {
		t.Error("pesan organik terbaca sebagai iklan")
	}
	if _, err := ParseCloudWebhook([]byte("bukan json")); err == nil {
		t.Error("JSON rusak seharusnya error")
	}
}

func TestHashNormalization(t *testing.T) {
	if Hash256("  Rina ") != Hash256("rina") {
		t.Error("hash tidak dinormalisasi")
	}
	if FirstName("Rina Amelia") != "rina" {
		t.Error("FirstName")
	}
	if NormalizeCity("Tangerang Selatan") != "tangerangselatan" || NormalizeCity("Jakarta-Barat.") != "jakartabarat" {
		t.Error("NormalizeCity")
	}
}
