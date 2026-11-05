package core

import "testing"

func TestRuleMatches(t *testing.T) {
	r := Rule{Who: "admin", Op: "contains", Keywords: []string{"pembayaran diterima", " lunas "}}
	cases := []struct {
		who, text string
		want      bool
	}{
		{"admin", "Halo kak, Pembayaran Diterima ya Rp 350.000", true},
		{"admin", "sudah LUNAS", true},
		{"admin", "terima kasih", false},
		{"customer", "pembayaran diterima", false}, // salah pengirim
	}
	for _, c := range cases {
		if got := r.Matches(c.who, c.text); got != c.want {
			t.Errorf("Matches(%q,%q) = %v; want %v", c.who, c.text, got, c.want)
		}
	}
	eq := Rule{Who: "customer", Op: "equals", Keywords: []string{"booking"}}
	if !eq.Matches("customer", "  Booking ") || eq.Matches("customer", "mau booking") {
		t.Error("op equals harus cocok persis (tanpa memedulikan huruf besar/kecil dan spasi tepi)")
	}
	if (Rule{Who: "customer", Op: "contains", Keywords: []string{"", "  "}}).Matches("customer", "apa saja") {
		t.Error("kata kunci kosong tidak boleh cocok dengan semua pesan")
	}
}

func TestRuleValue(t *testing.T) {
	parse := Rule{ValueMode: "parse"}
	if v, need := parse.Value("Pembayaran diterima Rp 350.000"); need || v == nil || *v != 350000 {
		t.Errorf("parse: %v %v", v, need)
	}
	if v, need := parse.Value("pembayaran diterima"); !need || v != nil {
		t.Errorf("parse gagal harus needValue: %v %v", v, need)
	}
	fv := 99000.0
	if v, need := (Rule{ValueMode: "fixed", FixedValue: &fv}).Value("apa pun"); need || *v != 99000 {
		t.Error("fixed")
	}
	if v, need := (Rule{ValueMode: "none"}).Value("Rp 5.000"); need || v != nil {
		t.Error("none tidak boleh membaca nilai")
	}
}
