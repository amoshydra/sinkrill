package stream

import (
	"bytes"
	"testing"
)

func newTestClient(maxQueued int) *Client {
	return &Client{
		maxQueued: maxQueued,
		signal:    make(chan struct{}, 1),
		done:      make(chan struct{}),
	}
}

func patterned(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i*7 + 3)
	}
	return b
}

func TestClientQueueDropsOldest(t *testing.T) {
	c := newTestClient(1000)
	chunk := make([]byte, 400)
	for i := 0; i < 5; i++ {
		c.push(chunk)
	}

	if got := c.queued(); got > 1000 {
		t.Fatalf("queued bytes = %d, want <= 1000", got)
	}
	if c.drops == 0 {
		t.Fatal("expected old chunks to be dropped")
	}
}

// A chunk bigger than the whole queue must not be discarded: dropping it opens
// a silent gap. The newest audio is kept instead.
func TestClientKeepsTailOfOversizedChunk(t *testing.T) {
	c := newTestClient(1000)
	chunk := patterned(2000)
	c.push(chunk)

	if got := c.queued(); got != 1000 {
		t.Fatalf("queued bytes = %d, want 1000", got)
	}
	if c.drops != 1 {
		t.Fatalf("drops = %d, want 1", c.drops)
	}

	got, ok := c.pop()
	if !ok {
		t.Fatal("expected a queued chunk")
	}
	if want := chunk[len(chunk)-1000:]; !bytes.Equal(got, want) {
		t.Fatalf("kept the wrong part of the chunk: got %d bytes", len(got))
	}
}

func TestTailDropsWholeFrames(t *testing.T) {
	chunk := patterned(2000)
	got := tail(chunk, 999)

	// Needs to drop 1001 bytes, rounded up to 1004 so the stream stays on a
	// frame boundary.
	if len(got) != 996 {
		t.Fatalf("kept %d bytes, want 996", len(got))
	}
	if !bytes.Equal(got, chunk[1004:]) {
		t.Fatal("kept the wrong slice")
	}
}

func TestTailLeavesFittingChunksAlone(t *testing.T) {
	chunk := patterned(500)
	got := tail(chunk, 1000)
	if len(got) != len(chunk) || !bytes.Equal(got, chunk) {
		t.Fatal("a chunk that fits should be returned unchanged")
	}
}

func TestQueueNeverExceedsLimit(t *testing.T) {
	c := newTestClient(1000)
	for i := 0; i < 3; i++ {
		c.push(patterned(2000))
	}
	if got := c.queued(); got > 1000 {
		t.Fatalf("queued bytes = %d, want <= 1000", got)
	}
}
