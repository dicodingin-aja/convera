package server

import "strings"

// customerHint memberi penjelasan dan saran untuk customer berdasarkan events.error_code
// (kode dari dispatcher, atau "meta_<kode>" dari Meta). Bahasa untuk pemilik bisnis, bukan teknisi.
func customerHint(code string) string {
	switch {
	case code == "":
		return ""
	case code == "not_from_ad":
		return "Chat ini tidak berasal dari iklan Click-to-WhatsApp, jadi Meta tidak bisa mengaitkannya ke iklan. Tidak ada yang perlu diperbaiki."
	case code == "attribution_expired":
		return "Meta hanya menerima konversi maksimal 7 hari setelah klik iklan. Event ini tidak bisa dikirim ulang. Tandai Purchase lebih cepat agar tidak terlewat."
	case code == "no_connection":
		return "Pixel/Dataset ID atau access token belum lengkap. Lengkapi di halaman Koneksi, lalu klik Kirim ulang."
	case code == "decrypt_failed":
		return "Token yang tersimpan tidak bisa dibaca. Simpan ulang access token di halaman Koneksi, lalu klik Kirim ulang."
	case code == "network":
		return "Gangguan jaringan ke Meta. Convera mencoba lagi otomatis; Anda tidak perlu melakukan apa-apa."
	case code == "build_failed":
		return "Data event belum lengkap atau tidak cocok dengan mode koneksi Anda. Periksa data lead, atau hubungi dukungan Convera."
	case code == "meta_190":
		return "Access token Meta tidak valid atau sudah dicabut. Buat token baru di Events Manager, simpan di halaman Koneksi, lalu klik Kirim ulang."
	case code == "meta_100", code == "meta_803", code == "meta_10", code == "meta_200":
		return "Meta menolak event ini. Penyebab umum: Dataset ID salah, token tidak punya akses ke dataset itu, atau dataset belum terhubung ke akun WhatsApp Business di Events Manager. Periksa halaman Koneksi."
	case code == "meta_4", code == "meta_17", code == "meta_32", code == "meta_613", code == "meta_1", code == "meta_2":
		return "Meta sedang membatasi permintaan atau mengalami gangguan sementara. Convera mencoba lagi otomatis; Anda tidak perlu melakukan apa-apa."
	case strings.HasPrefix(code, "meta_"):
		return "Meta mengembalikan error yang belum dikenali. Hubungi dukungan Convera dengan menyebut fbtrace_id pada detail event."
	}
	return ""
}
