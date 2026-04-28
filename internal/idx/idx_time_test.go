package idx

import (
	"encoding/json"
	"testing"
	"time"
)

func TestJsonTimeUnmarshalJSON(t *testing.T) {
	jakarta, _ := time.LoadLocation("Asia/Jakarta")

	tests := []struct {
		name    string
		input   string
		want    time.Time
		wantErr bool
	}{
		{
			name:    "valid time without timezone",
			input:   `"2026-04-14T21:58:23"`,
			want:    time.Date(2026, 4, 14, 21, 58, 23, 0, jakarta),
			wantErr: false,
		},
		{
			name:    "valid time with seconds",
			input:   `"2026-01-02T15:04:05"`,
			want:    time.Date(2026, 1, 2, 15, 4, 5, 0, jakarta),
			wantErr: false,
		},
		{
			name:    "empty string returns zero",
			input:   `""`,
			want:    time.Time{},
			wantErr: false,
		},
		{
			name:    "invalid format",
			input:   `"2026-04-14"`,
			want:    time.Time{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var jt JsonTime
			err := json.Unmarshal([]byte(tt.input), &jt)
			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			got := time.Time(jt)
			if !tt.wantErr {
				got = got.In(jakarta)
			}
			if got.Unix() != tt.want.Unix() {
				t.Errorf("UnmarshalJSON() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestJsonTimeTime(t *testing.T) {
	jakarta, _ := time.LoadLocation("Asia/Jakarta")
	jt := JsonTime(time.Date(2026, 4, 14, 21, 58, 23, 0, jakarta))
	got := jt.Time()
	want := time.Date(2026, 4, 14, 21, 58, 23, 0, jakarta)
	if !got.Equal(want) {
		t.Errorf("Time() = %v, want %v", got, want)
	}
}
