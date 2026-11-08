package stream

import (
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/amoshydra/thrum/internal/logx"
)

type Client struct {
	ID    int
	Label string

	conn net.Conn
	hub  *Hub
	log  *logx.Logger

	mu          sync.Mutex
	queue       [][]byte
	queuedBytes int
	maxQueued   int
	drops       int64

	signal    chan struct{}
	done      chan struct{}
	closeOnce sync.Once
	sent      atomic.Int64
}

// push enqueues a chunk, dropping the oldest audio when the client falls
// behind. Live audio must never accumulate: a slow listener should skip, not
// drift seconds behind.
func (c *Client) push(chunk []byte) {
	c.mu.Lock()
	if len(chunk) > c.maxQueued {
		c.drops++
		c.mu.Unlock()
		return
	}
	for len(c.queue) > 0 && c.queuedBytes+len(chunk) > c.maxQueued {
		c.queuedBytes -= len(c.queue[0])
		c.queue = c.queue[1:]
		c.drops++
	}
	c.queue = append(c.queue, chunk)
	c.queuedBytes += len(chunk)
	c.mu.Unlock()

	select {
	case c.signal <- struct{}{}:
	default:
	}
}

func (c *Client) pop() ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.queue) == 0 {
		return nil, false
	}
	chunk := c.queue[0]
	c.queue = c.queue[1:]
	c.queuedBytes -= len(chunk)
	return chunk, true
}

func (c *Client) run() {
	defer c.conn.Close()
	for {
		if chunk, ok := c.pop(); ok {
			_ = c.conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
			n, err := c.conn.Write(chunk)
			if n > 0 {
				c.sent.Add(int64(n))
			}
			if err != nil {
				c.hub.Remove(c)
				return
			}
			continue
		}

		select {
		case <-c.signal:
		case <-c.done:
			return
		}
	}
}

func (c *Client) close() {
	c.closeOnce.Do(func() {
		close(c.done)
		_ = c.conn.Close()
	})
}

func (c *Client) outstanding() int64 {
	c.mu.Lock()
	queued := c.queuedBytes
	c.mu.Unlock()
	return c.sent.Load() + int64(queued)
}

func (c *Client) queued() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return int64(c.queuedBytes)
}

type Hub struct {
	mu          sync.RWMutex
	clients     map[*Client]struct{}
	warmup      [][]byte
	warmupBytes int
	warmupLimit int
	maxQueued   int
	nextID      int
	total       atomic.Int64

	log *logx.Logger
}

// NewHub creates a fan-out hub. warmupMS > 0 replays recent audio to new
// listeners (higher latency, more resilient); 0 starts at live.
//
// bytesPerSecond is used to size both the warm-up ring and the per-client
// queue. The queue is capped at ~125 ms of audio so a slow client cannot
// build up seconds of delay.
func NewHub(warmupMS, bytesPerSecond int, log *logx.Logger) *Hub {
	warmupLimit := 0
	if warmupMS > 0 {
		warmupLimit = bytesPerSecond * warmupMS / 1000
		if warmupLimit < 4096 {
			warmupLimit = 4096
		}
	}

	maxQueued := bytesPerSecond / 8
	if maxQueued < 16*1024 {
		maxQueued = 16 * 1024
	}
	if maxQueued > 256*1024 {
		maxQueued = 256 * 1024
	}

	return &Hub{
		clients:     map[*Client]struct{}{},
		warmupLimit: warmupLimit,
		maxQueued:   maxQueued,
		log:         log,
	}
}

// Write fans one encoded chunk out to every listener. The chunk must not be
// mutated after this call; listeners share it.
func (h *Hub) Write(chunk []byte) {
	h.total.Add(int64(len(chunk)))

	if h.warmupLimit > 0 {
		h.mu.Lock()
		h.warmup = append(h.warmup, chunk)
		h.warmupBytes += len(chunk)
		for h.warmupBytes > h.warmupLimit && len(h.warmup) > 1 {
			h.warmupBytes -= len(h.warmup[0])
			h.warmup = h.warmup[1:]
		}
		h.mu.Unlock()
	}

	h.mu.RLock()
	for c := range h.clients {
		c.push(chunk)
	}
	h.mu.RUnlock()
}

// Add registers a connection and starts its writer goroutine.
func (h *Hub) Add(conn net.Conn, label string) *Client {
	h.mu.Lock()
	h.nextID++
	c := &Client{
		ID:        h.nextID,
		Label:     label,
		conn:      conn,
		hub:       h,
		log:       h.log,
		maxQueued: h.maxQueued,
		signal:    make(chan struct{}, 1),
		done:      make(chan struct{}),
	}
	h.clients[c] = struct{}{}
	if h.warmupLimit > 0 {
		for _, chunk := range h.warmup {
			c.push(chunk)
		}
	}
	count := len(h.clients)
	h.mu.Unlock()

	h.log.Infof("%s listening (%d active)", label, count)
	go c.run()
	return c
}

func (h *Hub) Remove(c *Client) {
	h.mu.Lock()
	_, present := h.clients[c]
	if present {
		delete(h.clients, c)
	}
	count := len(h.clients)
	h.mu.Unlock()

	if !present {
		return
	}
	c.close()
	h.log.Infof("listener %d disconnected (%d active)", c.ID, count)
}

func (h *Hub) CloseAll() {
	h.mu.Lock()
	clients := make([]*Client, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
		delete(h.clients, c)
	}
	h.mu.Unlock()

	for _, c := range clients {
		c.close()
	}
}

func (h *Hub) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

func (h *Hub) TotalBytes() int64 {
	return h.total.Load()
}

// MaxSentBytes is the number of media bytes written to the furthest-along
// listener.
func (h *Hub) MaxSentBytes() int64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	var max int64
	for c := range h.clients {
		if v := c.sent.Load(); v > max {
			max = v
		}
	}
	return max
}

// MaxOutstandingBytes is sent + queued for the furthest-along listener. This
// is what the latency probe must use: bytes still in our queue have not been
// played yet.
func (h *Hub) MaxOutstandingBytes() int64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	var max int64
	for c := range h.clients {
		if v := c.outstanding(); v > max {
			max = v
		}
	}
	return max
}

// MaxQueuedBytes is the largest per-client queue, in bytes.
func (h *Hub) MaxQueuedBytes() int64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	var max int64
	for c := range h.clients {
		if v := c.queued(); v > max {
			max = v
		}
	}
	return max
}

// TotalDrops counts chunks discarded for falling behind.
func (h *Hub) TotalDrops() int64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	var total int64
	for c := range h.clients {
		c.mu.Lock()
		total += c.drops
		c.mu.Unlock()
	}
	return total
}
