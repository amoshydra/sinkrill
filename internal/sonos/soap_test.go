package sonos

import (
	"encoding/xml"
	"testing"
)

const transportInfoXML = `<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
  <s:Body>
    <u:GetTransportInfoResponse xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">
      <CurrentTransportState>PLAYING</CurrentTransportState>
      <CurrentTransportStatus>OK</CurrentTransportStatus>
      <CurrentSpeed>1</CurrentSpeed>
    </u:GetTransportInfoResponse>
  </s:Body>
</s:Envelope>`

const positionInfoXML = `<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
  <s:Body>
    <u:GetPositionInfoResponse xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">
      <Track>1</Track>
      <TrackDuration>0:02:34</TrackDuration>
      <TrackURI>http://192.168.0.29:8567/stream.wav</TrackURI>
      <RelTime>0:01:02</RelTime>
    </u:GetPositionInfoResponse>
  </s:Body>
</s:Envelope>`

func TestParseTransportInfo(t *testing.T) {
	var parsed transportInfoResponse
	if err := xml.Unmarshal([]byte(transportInfoXML), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.State != "PLAYING" {
		t.Fatalf("state = %q, want PLAYING", parsed.State)
	}
}

func TestParsePositionInfo(t *testing.T) {
	var parsed positionInfoResponse
	if err := xml.Unmarshal([]byte(positionInfoXML), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.RelTime != "0:01:02" {
		t.Errorf("RelTime = %q", parsed.RelTime)
	}
	if parsed.TrackDuration != "0:02:34" {
		t.Errorf("TrackDuration = %q", parsed.TrackDuration)
	}
	if got := ParseRelTime(parsed.RelTime); got != 62 {
		t.Errorf("seconds = %d, want 62", got)
	}
	if parsed.TrackURI != "http://192.168.0.29:8567/stream.wav" {
		t.Errorf("TrackURI = %q", parsed.TrackURI)
	}
}
