// Package api: in-process per-IP token buckets (SHA-256 keys, memory only).
package api

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"time"
)

// bucket is one token bucket.
type bucket struct {
	tokens float64
	last   time.Time
}

// Limiter holds per-key token buckets with idle eviction.
type Limiter struct {
	mu        sync.Mutex
	buckets   map[string]*bucket
	capacity  float64
	perSec    float64
	idleTTL   time.Duration
	lastSweep time.Time
}

// NewLimiter builds a limiter allowing rate events/sec with burst capacity.
func NewLimiter(capacity, perSec float64) *Limiter {
	return &Limiter{
		buckets:   map[string]*bucket{},
		capacity:  capacity,
		perSec:    perSec,
		idleTTL:   time.Hour,
		lastSweep: time.Now(),
	}
}

// HashKey derives the bucket key as SHA-256(ip + salt) hex. Raw IPs are never
// used as keys or stored.
func HashKey(ip, salt string) string {
	sum := sha256.Sum256([]byte(ip + salt))
	return fmt.Sprintf("%x", sum[:])
}

// Allow reports whether key may proceed, consuming one token. RetryAfter is
// the seconds to wait when denied.
func (l *Limiter) Allow(key string) (bool, int) {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.lastSweep) > 5*time.Minute {
		for k, b := range l.buckets {
			if now.Sub(b.last) > l.idleTTL {
				delete(l.buckets, k)
			}
		}
		l.lastSweep = now
	}
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.capacity, last: now}
		l.buckets[key] = b
	}
	elapsed := now.Sub(b.last).Seconds()
	b.tokens += elapsed * l.perSec
	if b.tokens > l.capacity {
		b.tokens = l.capacity
	}
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	deficit := 1 - b.tokens
	wait := int(deficit/l.perSec) + 1
	if wait < 1 {
		wait = 1
	}
	return false, wait
}

// Size reports the number of tracked keys (for tests).
func (l *Limiter) Size() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}
