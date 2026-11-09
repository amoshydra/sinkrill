package virtualsink

import "testing"

func TestParseModuleID(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"12345\n", 12345},
		{"7", 7},
		{"  42  ", 42},
		{"", 0},
		{"not-a-number", 0},
		{"0", 0},
		{"-3", 0},
	}
	for _, tc := range tests {
		if got := parseModuleID(tc.in); got != tc.want {
			t.Errorf("parseModuleID(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}
