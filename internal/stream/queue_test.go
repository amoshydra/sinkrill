package stream

import "testing"

func TestClientQueueDropsOldest(t *testing.T) {
	c := &Client{
		maxQueued: 1000,
		signal:    make(chan struct{}, 1),
		done:      make(chan struct{}),
	}
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

func TestClientDropsOversizedChunk(t *testing.T) {
	c := &Client{
		maxQueued: 1000,
		signal:    make(chan struct{}, 1),
		done:      make(chan struct{}),
	}
	c.push(make([]byte, 2000))
	if got := c.queued(); got != 0 {
		t.Fatalf("queued bytes = %d, want 0", got)
	}
	if c.drops != 1 {
		t.Fatalf("drops = %d, want 1", c.drops)
	}
}
