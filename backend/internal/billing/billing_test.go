package billing

import (
	"testing"
	"time"
)

func d(s string) time.Time {
	t, err := time.Parse("2006-01-02 15:04", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestAddMonthsClamp(t *testing.T) {
	cases := []struct {
		in, want string
		n        int
	}{
		{"2026-01-31 10:00", "2026-02-28 10:00", 1}, // bukan 3 Maret
		{"2028-01-31 10:00", "2028-02-29 10:00", 1}, // kabisat
		{"2026-11-05 10:00", "2026-12-05 10:00", 1},
		{"2026-11-05 10:00", "2027-02-05 10:00", 3},
		{"2026-11-30 10:00", "2027-11-30 10:00", 12},
		{"2026-03-31 10:00", "2026-06-30 10:00", 3},
	}
	for _, c := range cases {
		if got := AddMonths(d(c.in), c.n); !got.Equal(d(c.want)) {
			t.Errorf("AddMonths(%s,%d) = %v; want %s", c.in, c.n, got, c.want)
		}
	}
}

func TestNextPeriod(t *testing.T) {
	now := d("2026-11-05 10:00")
	// masih aktif: sambung dari tanggal berakhir
	end := d("2026-11-20 10:00")
	s, e := NextPeriod(now, &end, 1)
	if !s.Equal(end) || !e.Equal(d("2026-12-20 10:00")) {
		t.Errorf("aktif: %v - %v", s, e)
	}
	// sudah lewat: mulai dari sekarang
	old := d("2026-10-01 10:00")
	s, e = NextPeriod(now, &old, 3) // 3 x 30 hari
	if !s.Equal(now) || !e.Equal(d("2027-02-03 10:00")) {
		t.Errorf("lewat: %v - %v", s, e)
	}
	// belum pernah berlangganan
	s, _ = NextPeriod(now, nil, 1)
	if !s.Equal(now) {
		t.Error("nil harus mulai dari sekarang")
	}
	// 1 bulan = 30 hari, termasuk melewati Februari; 12 bulan = 360 hari
	jan := d("2027-01-31 10:00")
	if _, e = NextPeriod(jan, nil, 1); !e.Equal(d("2027-03-02 10:00")) {
		t.Errorf("31 Jan + 30 hari = %v; want 2 Mar", e)
	}
	if _, e = NextPeriod(now, nil, 12); !e.Equal(now.AddDate(0, 0, 360)) {
		t.Errorf("12 bulan = %v; want +360 hari", e)
	}
}

func TestProofPrice(t *testing.T) {
	cases := []struct {
		pm, p12       float64
		months, slots int
		want          float64
	}{
		{150000, 1500000, 1, 1, 150000},
		{150000, 1500000, 1, 3, 450000},
		{150000, 1500000, 3, 2, 900000},
		{150000, 1500000, 12, 2, 3000000}, // harga 12 bulan per slot
		{150000, 0, 12, 1, 1800000},       // tanpa harga 12 bulan
	}
	for _, c := range cases {
		if got := ProofPrice(c.pm, c.p12, c.months, c.slots); got != c.want {
			t.Errorf("ProofPrice(%v,%v,%d,%d) = %v; want %v", c.pm, c.p12, c.months, c.slots, got, c.want)
		}
	}
}

func TestValidMonths(t *testing.T) {
	for _, n := range []int{1, 3, 12} {
		if !ValidMonths(n) {
			t.Errorf("%d seharusnya valid", n)
		}
	}
	for _, n := range []int{0, 2, 6, 24, -1} {
		if ValidMonths(n) {
			t.Errorf("%d seharusnya tidak valid", n)
		}
	}
}

func TestSplit(t *testing.T) {
	// Contoh dokumen: Rp150.000 -> komisi 15.000, kas 15.000, sisa 120.000 / 3 founder = 40.000
	s := Split(150000, 10, 10, 3)
	if s.Komisi != 15000 || s.Kas != 15000 || s.Each != 40000 {
		t.Errorf("Split(150000) = %+v", s)
	}
	// Pembulatan masuk kas: 100.001 -> komisi 10.000, kas0 10.000, sisa 80.001 / 3 = 26.667 sisa 0? 3*26667=80001
	s = Split(100002, 10, 10, 3) // komisi 10000, kas 10000, rest 80002, each 26667, sisa 1
	if s.Komisi != 10000 || s.Each != 26667 || s.Kas != 10000+1 {
		t.Errorf("pembulatan: %+v", s)
	}
	// Total selalu utuh
	for _, amt := range []int64{1, 7, 99999, 1500000, 1234567} {
		for _, f := range []int{0, 1, 3, 4} {
			s := Split(amt, 10, 10, f)
			if total := s.Komisi + s.Kas + s.Each*int64(f); total != amt {
				t.Errorf("Split(%d,f=%d) jumlah %d != %d (%+v)", amt, f, total, amt, s)
			}
		}
	}
	// Tanpa founder: sisa masuk kas
	if s := Split(1000, 10, 10, 0); s.Komisi != 100 || s.Kas != 900 || s.Each != 0 {
		t.Errorf("tanpa founder: %+v", s)
	}
	// Persentase pecahan
	if s := Split(100000, 10.5, 10, 3); s.Komisi != 10500 {
		t.Errorf("pecahan: %+v", s)
	}
}
