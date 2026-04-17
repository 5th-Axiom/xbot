//go:build gui

package main

import "sync"

// RingBuffer is a thread-safe fixed-capacity circular buffer for log lines.
type RingBuffer struct {
	mu    sync.RWMutex
	buf   []string
	cap   int
	write int
	count int
}

// NewRingBuffer creates a ring buffer with the given capacity.
func NewRingBuffer(capacity int) *RingBuffer {
	return &RingBuffer{
		buf: make([]string, capacity),
		cap: capacity,
	}
}

// Add appends a line to the buffer, overwriting the oldest if full.
func (rb *RingBuffer) Add(line string) {
	rb.mu.Lock()
	rb.buf[rb.write] = line
	rb.write = (rb.write + 1) % rb.cap
	if rb.count < rb.cap {
		rb.count++
	}
	rb.mu.Unlock()
}

// Last returns the most recent n lines in chronological order.
func (rb *RingBuffer) Last(n int) []string {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if n > rb.count {
		n = rb.count
	}
	if n == 0 {
		return nil
	}

	result := make([]string, n)
	start := (rb.write - n + rb.cap) % rb.cap
	for i := 0; i < n; i++ {
		result[i] = rb.buf[(start+i)%rb.cap]
	}
	return result
}

// Count returns the number of lines currently stored.
func (rb *RingBuffer) Count() int {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.count
}
