package idx

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/alturino/bloodhound/internal/models"
	"github.com/alturino/bloodhound/internal/store"
)

func TestAttachmentPathCleaner_Clean(t *testing.T) {
	cleaner := NewAttachmentPathCleaner()

	data, err := os.ReadFile("../../dummy/announcement_1.json")
	if err != nil {
		t.Fatalf("failed to read dummy file: %v", err)
	}

	var rawResp rawAnnouncementResponse
	if err := json.Unmarshal(data, &rawResp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	tests := []struct {
		name     string
		task     store.AttachmentTask
		wantPath string
	}{
		{
			name: "DMND_Laporan_Bulanan",
			task: store.AttachmentTask{
				AnnouncementID:    "20260507132617-DU/L-039/IDX/V/2026_id-id",
				AnnouncementTitle: "Laporan Bulanan Registrasi Pemegang Efek",
				StockCode:         "DMND",
				Date:              time.Date(2026, 5, 7, 0, 0, 0, 0, jakarta),
				Attachment: models.Attachment{
					OriginalFilename: "20260506_DMND_Laporan Bulanan Registrasi Pemegang Efek//Perubahan Struktur Pemegang Saham_32077778.pdf",
				},
			},
			wantPath: "dmnd/2026-05-07_laporan_bulanan_registrasi_pemegang_efek/2026-05-07_20260506_DMND_Laporan_Bulanan_Registrasi_Pemegang_Efek Perubahan_Struktur_Pemegang_Saham_32077778.pdf",
		},
		{
			name: "RSCH_Bukti_Iklan",
			task: store.AttachmentTask{
				AnnouncementID:    "20260507132308-053/PTCH/SKLR-CORSEC/V/2026_id-id",
				AnnouncementTitle: "Penyampaian Bukti Iklan Pemberitahuan RUPS",
				StockCode:         "RSCH",
				Date:              time.Date(2026, 5, 7, 0, 0, 0, 0, jakarta),
				Attachment: models.Attachment{
					OriginalFilename: "20260507_RSCH_Penyampaian Bukti Iklan_32078495.pdf",
				},
			},
			wantPath: "rsch/2026-05-07_penyampaian_bukti_iklan_pemberitahuan_rups/2026-05-07_20260507_RSCH_Penyampaian_Bukti_Iklan_32078495.pdf",
		},
		{
			name: "JTPE_Pemanggilan_RUPS",
			task: store.AttachmentTask{
				AnnouncementID:    "20260507132157-157/JTP/ACC/CS/V/2026_id-id",
				AnnouncementTitle: "Pemanggilan Rapat Umum Pemegang Saham Tahunan",
				StockCode:         "JTPE",
				Date:              time.Date(2026, 5, 7, 0, 0, 0, 0, jakarta),
				Attachment: models.Attachment{
					OriginalFilename: "20260507_JTPE_Pemanggilan RUPS_32078522.pdf",
				},
			},
			wantPath: "jtpe/2026-05-07_pemanggilan_rapat_umum_pemegang_saham_tahunan/2026-05-07_20260507_JTPE_Pemanggilan_RUPS_32078522.pdf",
		},
		{
			name: "TAMA_Rencana_RUPS",
			task: store.AttachmentTask{
				AnnouncementID:    "20260507130905-023/LTS-CORSEC/V/2026_id-id",
				AnnouncementTitle: "Pemberitahuan Rencana Rapat Umum Pemegang Saham Tahunan dan Luar Biasa",
				StockCode:         "TAMA",
				Date:              time.Date(2026, 5, 7, 0, 0, 0, 0, jakarta),
				Attachment: models.Attachment{
					OriginalFilename: "20260507_TAMA_Pengumuman RUPS_32078298.pdf",
				},
			},
			wantPath: "tama/2026-05-07_pemberitahuan_rencana_rapat_umum_pemegang_saham_tahunan_dan_luar_biasa/2026-05-07_20260507_TAMA_Pengumuman_RUPS_32078298.pdf",
		},
		{
			name: "ADES_Volatilitas",
			task: store.AttachmentTask{
				AnnouncementID:    "20260507120236-089/ER/LEG-SRT/AWI/V/2026_id-id",
				AnnouncementTitle: "Penjelasan atas Volatilitas Transaksi",
				StockCode:         "ADES",
				Date:              time.Date(2026, 5, 7, 0, 0, 0, 0, jakarta),
				Attachment: models.Attachment{
					OriginalFilename: "20260507_ADES_Tanggapan atas Permintaan Penjelasan Bursa_32078488.pdf",
				},
			},
			wantPath: "ades/2026-05-07_penjelasan_atas_volatilitas_transaksi/2026-05-07_20260507_ADES_Tanggapan_atas_Permintaan_Penjelasan_Bursa_32078488.pdf",
		},
		{
			name: "XPFT_Laporan_Unit_Penyertaan",
			task: store.AttachmentTask{
				AnnouncementID:    "20260507120408-07-A/XPFT/V/2026_id-id",
				AnnouncementTitle: "Laporan Jumlah Peradaran Unit Penyertaan",
				StockCode:         "XPFT",
				Date:              time.Date(2026, 5, 7, 0, 0, 0, 0, jakarta),
				Attachment: models.Attachment{
					OriginalFilename: "20260506_XPFT_Laporan Jumlah Peradaran Unit Penyertaan_32077996.pdf",
				},
			},
			wantPath: "xpft/2026-05-07_laporan_jumlah_peradaran_unit_penyertaan/2026-05-07_20260506_XPFT_Laporan_Jumlah_Peradaran_Unit_Penyertaan_32077996.pdf",
		},
		{
			name: "Long_Indonesian_Title_With_Colons_Semicolons",
			task: store.AttachmentTask{
				AnnouncementID:    "20260508000001-001/XXX/V/2026_id-id",
				AnnouncementTitle: "Penandatanganan: 1.\tAmandemen dan Pernyataan Kembali atas Perjanjian Investasi antara Perseroan, PT Aplikanusa Lintasarta (Lintasarta), PT Ainfrastruktur Indonesia Raya (Investor) tertanggal 6 Mei 2026 (A&R Perjanjian Investasi); 2.\tPerjanjian Jual Beli Saham Bersyarat (Conditional Sale and Purchase Agreement) antara Perseroan dan Lintasarta sebagai penjual (Para Penjual, atau masing-masing, Penjual) dan Perusahaan Baru (sebagaimana didefinisikan di bawah) sebagai pembeli (Pembeli) tertanggal 6 Mei 2026 (CSPA); dan 3.\tPerjanjian Pemegang Saham (Shareholders Agreement) antara Perseroan, Lintasarta dan Investor tertanggal 6 Mei 2026 (SHA).",
				StockCode:         "TEST",
				Date:              time.Date(2026, 5, 8, 0, 0, 0, 0, jakarta),
				Attachment: models.Attachment{
					OriginalFilename: "TEST_Document_32000001.pdf",
				},
			},
			wantPath: "test/2026-05-08_penandatanganan _1. amandemen_dan_pernyataan_kembali_atas_perjanjian_investasi_antara_perseroan _pt_aplikanusa_lintasarta_ linta/2026-05-08_TEST_Document_32000001.pdf",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleaner.Clean(t.Context(), tt.task)
			if got != tt.wantPath {
				t.Errorf("Clean() = %v, want %v", got, tt.wantPath)
			}
		})
	}
}
