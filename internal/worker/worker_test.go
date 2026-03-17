package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/alturino/bloodhound/internal/models"
)

func TestCalculateChecksum(t *testing.T) {
	data := []byte("hello world")
	expectedHash := sha256.Sum256(data)
	expected := hex.EncodeToString(expectedHash[:])

	actual := calculateChecksum(data)
	if actual != expected {
		t.Errorf("expected %s, got %s", expected, actual)
	}
}

func TestNamingLogic(t *testing.T) {
	ann := models.Announcement{
		StockCode:        "TLKM",
		AnnouncementDate: time.Date(2026, 3, 16, 17, 0, 0, 0, time.UTC),
	}
	originalFilename := "Financial_Report.PDF"
	data := []byte("some content")
	checksum := calculateChecksum(data)
	shortChecksum := checksum[:8]

	// Simulated naming logic from worker.go
	datePrefix := ann.AnnouncementDate.Format("2006-01-02")
	stockCode := "TLKM"                   // already trimmed and uppercase in test prep
	originalName := strings.ToLower(originalFilename) // lowercased
	
	actual := datePrefix + "_" + stockCode + "_" + shortChecksum + "_" + originalName
	expected := "2026-03-16_TLKM_" + shortChecksum + "_financial_report.pdf"

	if actual != expected {
		t.Errorf("expected %s, got %s", expected, actual)
	}
}
