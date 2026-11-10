package virtualsink

import "testing"

func TestOwnsSink(t *testing.T) {
	tests := []struct {
		args string
		name string
		want bool
	}{
		{"sink_name=sinkrill sink_properties=device.description=Sinkrill", "sinkrill", true},
		{"sink_name=sinkrill2 sink_properties=device.description=Sinkrill", "sinkrill", false},
		{"rate=48000 sink_name=sinkrill", "sinkrill", true},
		{"sink_name=sinkrill.2", "sinkrill", false},
		{"", "sinkrill", false},
	}
	for _, tc := range tests {
		if got := ownsSink(tc.args, tc.name); got != tc.want {
			t.Errorf("ownsSink(%q, %q) = %v, want %v", tc.args, tc.name, got, tc.want)
		}
	}
}
