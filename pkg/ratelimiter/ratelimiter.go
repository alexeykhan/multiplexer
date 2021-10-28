package ratelimiter

import (
	"sync"
)

type (
	RateLimiter interface {
		Done()
		Acquire() bool
		Release()
	}
	rateLimiter struct {
		window chan struct{}
		done   chan struct{}
		once   sync.Once
	}
)

// Interface compliance check.
var _ RateLimiter = (*rateLimiter)(nil)

// New returns a RateLimiter instance.
func New(limit uint64) RateLimiter {
	return &rateLimiter{
		window: make(chan struct{}, limit),
		done:   make(chan struct{}),
	}
}

// Done stops the observer.
func (rl *rateLimiter) Done() {
	rl.once.Do(func() {
		close(rl.done)
	})
}

// Acquire books a free spot in the window.
func (rl *rateLimiter) Acquire() bool {
	select {
	case <-rl.done:
		return false
	case rl.window <- struct{}{}:
		return true
	}
}

// Release releases a spot in the window.
func (rl *rateLimiter) Release() {
	<-rl.window
}
