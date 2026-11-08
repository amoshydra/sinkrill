package stream

import (
	"encoding/binary"
	"testing"
)

func TestWAVHeader(t *testing.T) {
	header := WAVHeader()
	if len(header) != 44 {
		t.Fatalf("header length = %d, want 44", len(header))
	}
	if got := string(header[0:4]); got != "RIFF" {
		t.Errorf("RIFF = %q", got)
	}
	if got := string(header[8:12]); got != "WAVE" {
		t.Errorf("WAVE = %q", got)
	}
	if got := string(header[12:16]); got != "fmt " {
		t.Errorf("fmt = %q", got)
	}
	if got := binary.LittleEndian.Uint16(header[20:22]); got != 1 {
		t.Errorf("audio format = %d, want 1 (PCM)", got)
	}
	if got := binary.LittleEndian.Uint16(header[22:24]); got != 2 {
		t.Errorf("channels = %d, want 2", got)
	}
	if got := binary.LittleEndian.Uint32(header[24:28]); got != 48000 {
		t.Errorf("sample rate = %d, want 48000", got)
	}
	if got := binary.LittleEndian.Uint32(header[28:32]); got != 192000 {
		t.Errorf("byte rate = %d, want 192000", got)
	}
	if got := binary.LittleEndian.Uint16(header[34:36]); got != 16 {
		t.Errorf("bits per sample = %d, want 16", got)
	}
	if got := string(header[36:40]); got != "data" {
		t.Errorf("data = %q", got)
	}
	if got := binary.LittleEndian.Uint32(header[40:44]); got != 0xffffffff {
		t.Errorf("data size = %#x, want 0xffffffff", got)
	}
}

func TestBytesPerSecond(t *testing.T) {
	if got := BytesPerSecond("wav", 192); got != 192000 {
		t.Errorf("wav bytes/sec = %d, want 192000", got)
	}
	if got := BytesPerSecond("mp3", 192); got != 24000 {
		t.Errorf("mp3 bytes/sec = %d, want 24000", got)
	}
}
