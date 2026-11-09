package sink

import "testing"

func TestStationForHost(t *testing.T) {
	tests := []struct {
		host string
		want string
	}{
		{"homedesktop", "homedesktop streaming audio"},
		{"homedesktop.lan", "homedesktop streaming audio"},
		{"desktop.example.com", "desktop streaming audio"},
		{"  padded  ", "padded streaming audio"},
		{"", "Sinkrill streaming audio"},
	}
	for _, tc := range tests {
		if got := stationForHost(tc.host); got != tc.want {
			t.Errorf("stationForHost(%q) = %q, want %q", tc.host, got, tc.want)
		}
	}
}

func TestDefaultStationIsNeverEmpty(t *testing.T) {
	if DefaultStation() == "" {
		t.Fatal("DefaultStation must not be empty; validate rejects an empty station")
	}
}
