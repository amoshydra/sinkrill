package main

import "testing"

func TestParseVolumeArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		mode    volumeMode
		value   int
		wantErr bool
	}{
		{"show", nil, volumeShow, 0, false},
		{"absolute", []string{"40"}, volumeSet, 40, false},
		{"zero", []string{"0"}, volumeSet, 0, false},
		{"hundred", []string{"100"}, volumeSet, 100, false},
		{"above range", []string{"101"}, 0, 0, true},
		{"negative absolute is relative", []string{"-5"}, volumeDelta, -5, false},
		{"explicit plus", []string{"+5"}, volumeDelta, 5, false},
		{"up default", []string{"up"}, volumeDelta, 5, false},
		{"down default", []string{"down"}, volumeDelta, -5, false},
		{"up step", []string{"up", "10"}, volumeDelta, 10, false},
		{"down step", []string{"down", "10"}, volumeDelta, -10, false},
		{"zero step", []string{"up", "0"}, 0, 0, true},
		{"mute", []string{"mute"}, volumeMute, 0, false},
		{"unmute", []string{"unmute"}, volumeUnmute, 0, false},
		{"garbage", []string{"loud"}, 0, 0, true},
		{"too many args", []string{"up", "5", "6"}, 0, 0, true},
		{"mute with step", []string{"mute", "5"}, 0, 0, true},
	}
	for _, tc := range tests {
		mode, value, err := parseVolumeArgs(tc.args)
		if tc.wantErr {
			if err == nil {
				t.Errorf("%s: expected an error", tc.name)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: unexpected error: %v", tc.name, err)
			continue
		}
		if mode != tc.mode || value != tc.value {
			t.Errorf("%s: got (%v, %d), want (%v, %d)", tc.name, mode, value, tc.mode, tc.value)
		}
	}
}

func TestClampVolume(t *testing.T) {
	cases := map[int]int{-20: 0, 0: 0, 55: 55, 100: 100, 140: 100}
	for in, want := range cases {
		if got := clampVolume(in); got != want {
			t.Errorf("clampVolume(%d) = %d, want %d", in, got, want)
		}
	}
}
