package stream

import (
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/amoshydra/sonos-sink/internal/logx"
)

// clientQueue bounds per-listener buffering. Live audio is dropped rather than
// queued forever when a client falls behind.
const clientQueue = 256

type Client struct {
	ID    int
	Label string

	conn  net.Conn
	ch    chan []byte
	done  chan struct{}
	once  sync.Once
	sent  atomic.Int64
	drops atomic.Int64

	hub *Hub
	log *logx.Logger
}

func (c *Client) run() {
	defer c.conn.Close()
	for {
		select {
		case chunk := <-c.ch:
			_ = c.conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
			n, err := c.conn.Write(chunk)
			if n > 0 {
				c.sent.Add(int64(n))
			}
			if err != nil {
				c.hub.Remove(c)
				return
			}
		case <-c.done:
			return
		}
	}
}

func (c *Client) close() {
	c.once.Do(func() {
		close(c.done)
		_ = c.conn.Close()
	})
}

type Hub struct {
	mu          sync.RWMutex
	clients     map[*Client]struct{}
	warmup      [][]byte
	warmupBytes int
	warmupLimit int
	nextID      int
	total       atomic.Int64

	log *logx.Logger
}

// NewHub creates a fan-out hub. warmupMS > 0 replays recent audio to new
// listeners (higher latency, more resilient); 0 starts at live.
func NewHub(warmupMS, bitrateKbps int, log *logx.Logger) *Hub {
	limit := 0
	if warmupMS > 0 {
		limit = bitrateKbps * 1000 / 8 * warmupMS / 1000
		if limit < 4096 {
			limit = 4096
		}
	}
	return &Hub{
		clients:     map[*Client]struct{}{},
		warmupLimit: limit,
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
		select {
		case c.ch <- chunk:
		default:
			c.drops.Add(1)
		}
	}
	h.mu.RUnlock()
}

// Add registers a connection and starts its writer goroutine.
func (h *Hub) Add(conn net.Conn, label string) *Client {
	h.mu.Lock()
	h.nextID++
	c := &Client{
		ID:    h.nextID,
		Label: label,
		conn:  conn,
		ch:    make(chan []byte, clientQueue),
		done:  make(chan struct{}),
		hub:   h,
		log:   h.log,
	}
	h.clients[c] = struct{}{}
	if h.warmupLimit > 0 {
		for _, chunk := range h.warmup {
			select {
			case c.ch <- chunk:
			default:
			}
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
// listener. Used by the latency probe.
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
