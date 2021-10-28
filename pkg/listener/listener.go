package listener

import (
	"net"
	"sync"

	"github.com/alexeykhan/multiplexer/pkg/ratelimiter"
)

type (
	listener struct {
		net.Listener
		ratelimiter.RateLimiter
	}
	connection struct {
		net.Conn
		once    sync.Once
		release func()
	}
)

// Interface compliance check.
var (
	_ net.Listener = (*listener)(nil)
	_ net.Conn     = (*connection)(nil)

	_ ratelimiter.RateLimiter = (*listener)(nil)
)

// New returns a net.Listener with built-in rate limiter for {limit} concurrent requests.
// A default net.Listener is returned if limit equals to zero.
func New(network, address string, limit uint16) (lstnr net.Listener, err error) {
	if lstnr, err = net.Listen(network, address); err != nil {
		return nil, err
	}
	if limit == 0 {
		return
	}
	return &listener{
		Listener:    lstnr,
		RateLimiter: ratelimiter.New(uint64(limit)),
	}, nil
}

// Accept waits for and returns the next connection to the listener.
func (rl *listener) Accept() (conn net.Conn, err error) {
	acquiredLock := rl.RateLimiter.Acquire()
	if conn, err = rl.Listener.Accept(); err != nil {
		if acquiredLock {
			rl.RateLimiter.Release()
		}
		return nil, err
	}
	return &connection{Conn: conn, release: rl.RateLimiter.Release}, nil
}

// Close closes the listener.
func (rl *listener) Close() error {
	err := rl.Listener.Close()
	rl.RateLimiter.Done()
	return err
}

// Close closes the connection.
func (cn *connection) Close() (err error) {
	err = cn.Conn.Close()
	cn.once.Do(cn.release)
	return
}
