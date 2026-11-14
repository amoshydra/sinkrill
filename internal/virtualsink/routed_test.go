package virtualsink

import "testing"

func TestCountInputs(t *testing.T) {
	list := "1286\t60\t1285\tPipeWire\ts16le 1ch 44100Hz\n" +
		"3157\t60\t2948\tPipeWire\tfloat32le 2ch 48000Hz\n" +
		"4283\t5289\t2948\tPipeWire\tfloat32le 2ch 48000Hz\n" +
		"4290\t52\t2948\tPipeWire\tfloat32le 2ch 44100Hz\n"

	tests := []struct {
		sinkIndex string
		want      int
	}{
		{"60", 2},
		{"5289", 1},
		{"52", 1},
		{"999", 0},
	}
	for _, tc := range tests {
		if got := countInputs(list, tc.sinkIndex); got != tc.want {
			t.Errorf("countInputs(sink %s) = %d, want %d", tc.sinkIndex, got, tc.want)
		}
	}

	if got := countInputs("", "60"); got != 0 {
		t.Errorf("empty list counted %d inputs", got)
	}
}
