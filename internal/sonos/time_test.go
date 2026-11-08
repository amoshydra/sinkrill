package sonos

import "testing"

func TestParseRelTime(t *testing.T) {
	cases := map[string]int{
		"0:00:00":  0,
		"0:00:07":  7,
		"0:01:00":  60,
		"1:02:03":  3723,
		" 0:00:12": 12,
		"bad":      0,
		"":         0,
	}
	for input, want := range cases {
		if got := ParseRelTime(input); got != want {
			t.Errorf("ParseRelTime(%q) = %d, want %d", input, got, want)
		}
	}
}
