// Package billing berisi aturan uang Convera yang murni (tanpa database): periode langganan dan bagi hasil.
// Semua nominal dalam Rupiah bulat (int64).
package billing

import "time"

// AddMonths menambah n bulan kalender. Bila tanggal tujuan tidak ada (31 Jan + 1 bulan), dipakai hari
// terakhir bulan tujuan (28/29 Feb), bukan melompat ke bulan berikutnya.
func AddMonths(t time.Time, n int) time.Time {
	y, m, d := t.Date()
	target := time.Date(y, m+time.Month(n), 1, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
	last := time.Date(target.Year(), target.Month()+1, 0, 0, 0, 0, 0, t.Location()).Day()
	if d > last {
		d = last
	}
	return time.Date(target.Year(), target.Month(), d, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
}

// DaysPerMonth: satu "bulan" langganan dihitung 30 hari (3 bulan = 90 hari, 12 bulan = 360 hari), sama dengan
// slot tambahan yang juga berlaku 30 hari.
const DaysPerMonth = 30

// NextPeriod menentukan periode yang dibeli. Bila langganan masih berjalan (currentEnd di masa depan),
// periode baru mulai dari currentEnd (sisa hari tidak hilang); selain itu mulai dari sekarang.
func NextPeriod(now time.Time, currentEnd *time.Time, months int) (start, end time.Time) {
	start = now
	if currentEnd != nil && currentEnd.After(now) {
		start = *currentEnd
	}
	return start, start.AddDate(0, 0, months*DaysPerMonth)
}

// MaxAdSlots: batas jumlah slot dalam satu pembelian.
const MaxAdSlots = 50

// ProofPrice menghitung tagihan: harga per slot per bulan x slot x bulan; 12 bulan memakai harga 12 bulan per slot
// bila diisi. Pembelian 'slots' (tambah slot di tengah periode) selalu 1 bulan (30 hari).
func ProofPrice(priceMonth, price12 float64, months, slots int) float64 {
	if months == 12 && price12 > 0 {
		return price12 * float64(slots)
	}
	return priceMonth * float64(months) * float64(slots)
}

// ValidMonths: periode yang boleh dipilih saat aktivasi (dokumen bab 8: 1, 3, atau 12 bulan).
func ValidMonths(n int) bool { return n == 1 || n == 3 || n == 12 }

// Shares adalah hasil pembagian satu pembayaran.
type Shares struct {
	Komisi int64 // untuk sales yang membawa customer
	Kas    int64 // kas bersama, termasuk sisa pembulatan
	Each   int64 // bagian rata tiap founder
}

// Split membagi satu pembayaran: komisi% ke sales, kas% ke kas bersama, sisanya dibagi rata ke
// founders orang. Semua pembulatan ke bawah; sisa pembulatan masuk kas. Persentase boleh berpecahan (10,5).
func Split(amount int64, komisiPct, kasPct float64, founders int) Shares {
	komisi := amount * bp(komisiPct) / 10000
	kas := amount * bp(kasPct) / 10000
	rest := amount - komisi - kas
	if founders <= 0 {
		return Shares{Komisi: komisi, Kas: kas + rest}
	}
	each := rest / int64(founders)
	return Shares{Komisi: komisi, Kas: kas + (rest - each*int64(founders)), Each: each}
}

func bp(pct float64) int64 { return int64(pct*100 + 0.5) } // basis poin
