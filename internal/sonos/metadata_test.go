package sonos

import (
	"strings"
	"testing"
)

func TestStreamURI(t *testing.T) {
	if got := StreamURI("192.168.0.29", 8567, "wav"); got != "http://192.168.0.29:8567/stream.wav" {
		t.Errorf("wav URI = %q", got)
	}
	if got := StreamURI("192.168.0.29", 8567, "mp3"); got != "x-rincon-mp3radio://192.168.0.29:8567/stream.mp3" {
		t.Errorf("mp3 URI = %q", got)
	}
}

func TestWAVMetadata(t *testing.T) {
	meta := WAVMetadata(`Den & "Kitchen" <1>`, "http://x/stream.wav?a=1&b=2", "thrum")
	if !strings.Contains(meta, "object.item.audioItem.musicTrack") {
		t.Error("missing musicTrack class")
	}
	if !strings.Contains(meta, `protocolInfo="http-get:*:audio/wav:*"`) {
		t.Error("missing wav res protocolInfo")
	}
	if strings.Contains(meta, "<1>") {
		t.Error("title was not XML-escaped")
	}
	if !strings.Contains(meta, "Den &amp; &#34;Kitchen&#34; &lt;1&gt;") {
		t.Errorf("unexpected escaped title: %s", meta)
	}
	if !strings.Contains(meta, "a=1&amp;b=2") {
		t.Error("URI was not XML-escaped")
	}
}

func TestRadioMetadata(t *testing.T) {
	meta := RadioMetadata("Thrum", "thrum")
	if !strings.Contains(meta, "object.item.audioItem.audioBroadcast") {
		t.Error("missing audioBroadcast class")
	}
	if !strings.Contains(meta, "<dc:title>Thrum</dc:title>") {
		t.Error("missing title")
	}
}
