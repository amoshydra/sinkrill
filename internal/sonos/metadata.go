package sonos

import (
	"bytes"
	"encoding/xml"
	"fmt"
)

func escapeXML(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// StreamURI returns the URI Sonos should play. WAV uses a plain http URL so
// Sonos treats it as a file; MP3 uses the radio scheme to force streaming.
func StreamURI(lanIP string, port int, format string) string {
	if format == "wav" {
		return fmt.Sprintf("http://%s:%d/stream.wav", lanIP, port)
	}
	return fmt.Sprintf("x-rincon-mp3radio://%s:%d/stream.mp3", lanIP, port)
}

// RadioMetadata describes an internet-radio style MP3 stream.
func RadioMetadata(title, artist string) string {
	return `<DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/"` +
		` xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/"` +
		` xmlns:r="urn:schemas-rinconnetworks-com:metadata-1-0/"` +
		` xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/">` +
		`<item id="-1" parentID="-1" restricted="true">` +
		`<dc:title>` + escapeXML(title) + `</dc:title>` +
		`<dc:creator>` + escapeXML(artist) + `</dc:creator>` +
		`<upnp:class>object.item.audioItem.audioBroadcast</upnp:class>` +
		`<desc id="cdudn" nameSpace="urn:schemas-rinconnetworks-com:metadata-1-0/">` +
		`RINCON_AssociatedZPUDN</desc>` +
		`</item></DIDL-Lite>`
}

// WAVMetadata describes a file-like WAV stream, including a res element.
func WAVMetadata(title, uri, artist string) string {
	return `<DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/"` +
		` xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/"` +
		` xmlns:r="urn:schemas-rinconnetworks-com:metadata-1-0/"` +
		` xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/">` +
		`<item id="-1" parentID="-1" restricted="true">` +
		`<dc:title>` + escapeXML(title) + `</dc:title>` +
		`<dc:creator>` + escapeXML(artist) + `</dc:creator>` +
		`<upnp:class>object.item.audioItem.musicTrack</upnp:class>` +
		`<res protocolInfo="http-get:*:audio/wav:*">` + escapeXML(uri) + `</res>` +
		`<desc id="cdudn" nameSpace="urn:schemas-rinconnetworks-com:metadata-1-0/">` +
		`RINCON_AssociatedZPUDN</desc>` +
		`</item></DIDL-Lite>`
}
