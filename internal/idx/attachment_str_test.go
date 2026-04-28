package idx

import (
	"regexp"
	"strings"
	"testing"
)

func TestFileRename(t *testing.T) {
	filename := "Laporan Informasi atau Fakta Material Kesiapan Perusahaan untuk Pembayaran Pokok Obligasi Berkelanjutan V Pegadaian Tahap I Tahun 2022 Seri A, Sukuk Mudharabah Berkelanjutan II Pegadaian Tahap I Tahun 2022 Seri A, Obligasi Berkelanjutan IV Pegadaian Tahap I Tahun 2020 Seri B , dan Sukuk Mudharabah Berkelanjutan I Pegadaian Tahap I Tahun 2020 Seri B [PPGD ]"
	filename = strings.ToLower(filename)
	filename = regexp.MustCompile(`[^a-zA-Z0-9]+`).ReplaceAllString(filename, "_")
	filename = strings.Trim(filename, "_")
	exp := "laporan_informasi_atau_fakta_material_kesiapan_perusahaan_untuk_pembayaran_pokok_obligasi_berkelanjutan_v_pegadaian_tahap_i_tahun_2022_seri_a_sukuk_mudharabah_berkelanjutan_ii_pegadaian_tahap_i_tahun_2022_seri_a_obligasi_berkelanjutan_iv_pegadaian_tahap_i_tahun_2020_seri_b_dan_sukuk_mudharabah_berkelanjutan_i_pegadaian_tahap_i_tahun_2020_seri_b_ppgd"
	if filename != exp {
		t.Fatalf("string should be the same\nfilename=%s\nexp=%s", filename, exp)
	}
}
