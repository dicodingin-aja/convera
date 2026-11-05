package adminapi

import "strings"

// supportHint memberi saran tindakan untuk tim dukungan berdasarkan events.error_code.
// Kode berasal dari dispatcher (not_from_ad, attribution_expired, no_connection, decrypt_failed, network, build_failed)
// dan dari Meta ("meta_<kode>").
func supportHint(code string) string {
	switch {
	case code == "":
		return ""
	case code == "not_from_ad":
		return "Lead ini bukan dari iklan Click-to-WhatsApp (tidak ada ctwa_clid). Mode WABA hanya mengirim konversi dari iklan, jadi ini normal dan tidak perlu diperbaiki."
	case code == "attribution_expired":
		return "Konversi terjadi lebih dari 7 hari setelah klik iklan sehingga tidak dikirim ke Meta. Tidak bisa dikirim ulang."
	case code == "no_connection":
		return "Pixel/Dataset ID atau access token belum lengkap di halaman Koneksi customer. Minta customer melengkapinya, lalu kirim ulang event ini."
	case code == "decrypt_failed":
		return "Token tersimpan tidak bisa dibaca (kunci enkripsi server berubah atau data rusak). Minta customer menyimpan ulang token di halaman Koneksi."
	case code == "network":
		return "Gangguan jaringan ke Meta. Sistem mencoba ulang otomatis (1m, 5m, 30m, 2j, 6j); bila tetap gagal, kirim ulang dari sini."
	case code == "build_failed":
		return "Data event tidak lengkap atau mode koneksi tidak cocok dengan jenis event. Periksa lead dan mode koneksi customer."
	case code == "meta_190":
		return "Token Meta tidak valid atau sudah dicabut. Minta customer membuat token baru di Events Manager dan menyimpannya di halaman Koneksi, lalu kirim ulang."
	case code == "meta_100", code == "meta_803", code == "meta_10", code == "meta_200":
		return "Meta menolak permintaan: umumnya Dataset ID salah, token tidak punya akses ke dataset itu, atau jenis event tidak didukung WhatsApp resmi. Cek Dataset ID, izin token, dan hubungkan dataset ke akun WhatsApp Business di Events Manager."
	case code == "meta_4", code == "meta_17", code == "meta_32", code == "meta_613", code == "meta_1", code == "meta_2":
		return "Meta sedang membatasi laju atau mengalami gangguan sementara. Sistem mencoba ulang otomatis."
	case strings.HasPrefix(code, "meta_"):
		return "Kode error Meta tidak dikenal. Baca pesan error dan fbtrace_id pada detail event, lalu hubungi tim teknis bila perlu."
	}
	return ""
}
