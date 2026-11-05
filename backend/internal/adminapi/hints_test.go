package adminapi

import (
	"strings"
	"testing"
)

func TestSupportHint(t *testing.T) {
	for code, want := range map[string]string{
		"not_from_ad": "ctwa_clid", "attribution_expired": "7 hari", "no_connection": "Koneksi", "decrypt_failed": "menyimpan ulang",
		"network": "otomatis", "meta_190": "token baru", "meta_100": "Dataset ID", "meta_613": "membatasi laju", "meta_9999": "fbtrace_id",
	} {
		if h := supportHint(code); !strings.Contains(h, want) {
			t.Errorf("supportHint(%q) = %q; harus memuat %q", code, h, want)
		}
	}
	if supportHint("") != "" || supportHint("kode_baru_tak_dikenal") != "" {
		t.Error("kode kosong atau tak dikenal tidak boleh punya saran")
	}
}
