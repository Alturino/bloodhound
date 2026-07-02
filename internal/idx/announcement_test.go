package idx

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestIsPDF(t *testing.T) {
	cases := []struct {
		name string
		att  Attachment
		want bool
	}{
		{
			name: "original pdf lowercase",
			att:  Attachment{OriginalFilename: "laporan.pdf"},
			want: true,
		},
		{
			name: "original pdf uppercase",
			att:  Attachment{OriginalFilename: "laporan.PDF"},
			want: true,
		},
		{
			name: "original pdf mixed case",
			att:  Attachment{OriginalFilename: "laporan.Pdf"},
			want: true,
		},
		{
			name: "original zip",
			att:  Attachment{OriginalFilename: "instance.zip"},
			want: false,
		},
		{
			name: "original xlsx",
			att:  Attachment{OriginalFilename: "FinancialStatement.xlsx"},
			want: false,
		},
		{
			name: "empty original falls back to pdf filename",
			att:  Attachment{OriginalFilename: "", PDFFilename: "x.pdf"},
			want: true,
		},
		{
			name: "no extension original falls back to pdf filename",
			att:  Attachment{OriginalFilename: "no_extension", PDFFilename: "x.pdf"},
			want: true,
		},
		{
			name: "hidden file original falls back to pdf filename",
			att:  Attachment{OriginalFilename: ".hidden", PDFFilename: "x.pdf"},
			want: true,
		},
		{
			name: "non-pdf original but pdf filename wins",
			att:  Attachment{OriginalFilename: "instance.zip", PDFFilename: "x.pdf"},
			want: true,
		},
		{
			name: "both empty",
			att:  Attachment{},
			want: false,
		},
		{
			name: "no extension original only",
			att:  Attachment{OriginalFilename: "no_extension"},
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.att.isPDF(); got != tc.want {
				t.Errorf("isPDF(%+v) = %v, want %v", tc.att, got, tc.want)
			}
		})
	}
}

func TestToAttachments_DropsNonPDFs(t *testing.T) {
	a := Announcement{
		ID: "ann-1",
		Attachments: []Attachment{
			{PDFFilename: "a.pdf", OriginalFilename: "a.pdf"},
			{PDFFilename: "b.zip", OriginalFilename: "b.zip"},
			{PDFFilename: "c.xlsx", OriginalFilename: "c.xlsx"},
			{PDFFilename: "d.pdf", OriginalFilename: "d.PDF"},
		},
	}

	got := a.ToAttachments()
	if len(got) != 2 {
		t.Fatalf("ToAttachments length = %d, want 2", len(got))
	}
	wantNames := []string{"a.pdf", "d.PDF"}
	for i, want := range wantNames {
		if got[i].OriginalFilename != want {
			t.Errorf("ToAttachments[%d].OriginalFilename = %q, want %q", i, got[i].OriginalFilename, want)
		}
	}
}

func TestToAttachments_PrefersPDFFilename(t *testing.T) {
	a := Announcement{
		ID: "ann-1",
		Attachments: []Attachment{
			{PDFFilename: "x.pdf", OriginalFilename: "no_ext"},
		},
	}

	got := a.ToAttachments()
	if len(got) != 1 {
		t.Fatalf("ToAttachments length = %d, want 1", len(got))
	}
	if got[0].Filename != "x.pdf" {
		t.Errorf("ToAttachments[0].Filename = %q, want %q", got[0].Filename, "x.pdf")
	}
	if got[0].OriginalFilename != "no_ext" {
		t.Errorf("ToAttachments[0].OriginalFilename = %q, want %q", got[0].OriginalFilename, "no_ext")
	}
}

func TestConvertToModel_FiltersNonPDFs(t *testing.T) {
	rawJSON := `{
		"Replies": [
			{
				"pengumuman": {
					"Id2": "reply-1",
					"JudulPengumuman": "Test 1",
					"Kode_Emiten": "BBCA"
				},
				"attachments": [
					{"PDFFilename": "a.pdf", "FullSavePath": "u1", "OriginalFilename": "a.pdf"},
					{"PDFFilename": "b.zip", "FullSavePath": "u2", "OriginalFilename": "b.zip"},
					{"PDFFilename": "c.pdf", "FullSavePath": "u3", "OriginalFilename": "c.pdf"}
				]
			},
			{
				"pengumuman": {
					"Id2": "reply-2",
					"JudulPengumuman": "Test 2",
					"Kode_Emiten": "BBRI"
				},
				"attachments": [
					{"PDFFilename": "d.zip", "FullSavePath": "u4", "OriginalFilename": "d.zip"},
					{"PDFFilename": "e.pdf", "FullSavePath": "u5", "OriginalFilename": "e.PDF"},
					{"PDFFilename": "f.zip", "FullSavePath": "u6", "OriginalFilename": "f.zip"}
				]
			}
		]
	}`

	var raw rawAnnouncementResponse
	if err := json.Unmarshal([]byte(rawJSON), &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}

	got := convertToModel(raw)
	if len(got.Announcements) != 2 {
		t.Fatalf("Announcements length = %d, want 2", len(got.Announcements))
	}

	wantPerReply := []int{2, 1}
	wantIDs := []string{"reply-1", "reply-2"}
	for i, want := range wantPerReply {
		if got.Announcements[i].ID != wantIDs[i] {
			t.Errorf("Announcements[%d].ID = %q, want %q", i, got.Announcements[i].ID, wantIDs[i])
		}
		if len(got.Announcements[i].Attachments) != want {
			t.Errorf("Announcements[%d].Attachments length = %d, want %d", i, len(got.Announcements[i].Attachments), want)
		}
		for _, att := range got.Announcements[i].Attachments {
			if !strings.HasSuffix(strings.ToLower(att.OriginalFilename), ".pdf") {
				t.Errorf("Announcements[%d] kept non-pdf attachment: %q", i, att.OriginalFilename)
			}
		}
	}
}
