package closer

import (
	"log"
	"os"
	"os/signal"
	"sync"
)

type (
	Closer interface {
		Add(f ...func() error)
		Wait()
		Close()
	}
	closer struct {
		sync.Mutex
		once  sync.Once
		done  chan struct{}
		funcs []func() error
	}
)

// Interface compliance check.
var _ Closer = (*closer)(nil)

// NewCloser returns new Closer. If any os.Signal is specified, Closer will
// call Close when it receives one of the signals from the OS.
func New(sig ...os.Signal) Closer {
	c := &closer{done: make(chan struct{})}
	if len(sig) > 0 {
		go func() {
			ch := make(chan os.Signal, 1)
			signal.Notify(ch, sig...)
			stop := <-ch
			signal.Stop(ch)
			log.Printf("OS signal received: %s\n", stop.String())
			c.Close()
		}()
	}
	return c
}

// Add queues the function to run on close.
func (c *closer) Add(f ...func() error) {
	c.Lock()
	c.funcs = append(c.funcs, f...)
	c.Unlock()
}

// Wait blocks until all closer functions are done.
func (c *closer) Wait() {
	<-c.done
}

// Close calls all closer functions.
func (c *closer) Close() {
	c.once.Do(func() {
		defer close(c.done)

		c.Lock()
		funcs := c.funcs
		c.funcs = nil
		c.Unlock()

		errs := make(chan error, len(funcs))
		for _, f := range funcs {
			go func(f func() error) {
				errs <- f()
			}(f)
		}

		for i := 0; i < cap(errs); i++ {
			if err := <-errs; err != nil {
				log.Printf("closer: %s", err.Error())
			}
		}
	})
}
