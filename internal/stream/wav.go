package stream

import "encoding/binary"

const (
	WAVSampleRate = 48000
	WAVChannels   = 2
	WAVBits       = 16
)

// WAVHeader returns a 44-byte PCM WAV header that advertises an "unknown"
// length. Sonos plays this as a file stream, which skips its radio buffer.
func WAVHeader() []byte {
	const (
		headerSize = 44
		unknown    = 0xffffffff
	)
	blockAlign := uint16(WAVChannels * (WAVBits / 8))
	byteRate := uint32(WAVSampleRate) * uint32(blockAlign)

	b := make([]byte, headerSize)
	copy(b[0:4], "RIFF")
	binary.LittleEndian.PutUint32(b[4:8], unknown)
	copy(b[8:12], "WAVE")
	copy(b[12:16], "fmt ")
	binary.LittleEndian.PutUint32(b[16:20], 16)
	binary.LittleEndian.PutUint16(b[20:22], 1) // PCM
	binary.LittleEndian.PutUint16(b[22:24], WAVChannels)
	binary.LittleEndian.PutUint32(b[24:28], WAVSampleRate)
	binary.LittleEndian.PutUint32(b[28:32], byteRate)
	binary.LittleEndian.PutUint16(b[32:34], blockAlign)
	binary.LittleEndian.PutUint16(b[34:36], WAVBits)
	copy(b[36:40], "data")
	binary.LittleEndian.PutUint32(b[40:44], unknown)
	return b
}

// BytesPerSecond is the media rate of the active format.
func BytesPerSecond(format string, bitrateKbps int) int {
	if format == "wav" {
		return WAVSampleRate * WAVChannels * (WAVBits / 8)
	}
	return bitrateKbps * 1000 / 8
}
