package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"syscall"
	"time"

	"github.com/alexeykhan/multiplexer/pkg/closer"
	"github.com/alexeykhan/multiplexer/pkg/crawler"
	"github.com/alexeykhan/multiplexer/pkg/listener"
)

type (
	App interface {
		Run() error
	}
	Config struct {
		HTTPPort        uint16 // Public HTTP port.
		MaxConnections  uint16 // Number of simultaneous connections.
		GracefulDelay   time.Duration
		GracefulTimeout time.Duration
	}
	app struct {
		http struct {
			server   *http.ServeMux
			listener net.Listener
		}
		config  Config
		closer  closer.Closer
		crawler crawler.Crawler
	}
)

var (
	// Interface compliance check.
	_ App = (*app)(nil)

	// defaultConfig stores predefined settings.
	defaultConfig = Config{
		HTTPPort:        80,
		MaxConnections:  100,
		GracefulDelay:   3 * time.Second,
		GracefulTimeout: 3 * time.Second,
	}
)

// New creates a new App instance with default settings.
func New() (App, error) {
	return NewWithConfig(defaultConfig)
}

// NewWithConfig creates a new App instance with custom settings.
func NewWithConfig(cfg Config) (_ App, err error) {
	a := &app{config: cfg}

	// Init a closer.
	a.closer = closer.New(syscall.SIGTERM, syscall.SIGINT, os.Interrupt)

	// Set up handlers for routes.
	a.http.server = http.NewServeMux()
	a.http.server.Handle("/crawler", a.handler())

	// Init a crawler instance for reusable purposes.
	a.crawler = crawler.New()

	// Set up new listener.
	network, address := "tcp", fmt.Sprintf(":%d", a.config.HTTPPort)
	if a.http.listener, err = listener.New(network, address, a.config.MaxConnections); err != nil {
		return nil, fmt.Errorf("listen on tcp port %d: %w", a.config.HTTPPort, err)
	}

	return a, nil
}

// Run starts a server and sets shutdown handler.
func (a *app) Run() error {
	port := a.http.listener.Addr().(*net.TCPAddr).Port
	log.Printf("app started on port: %d\n", port)

	srv := &http.Server{Handler: a.http.server}
	go func() {
		if err := srv.Serve(a.http.listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("http: %s\n", err.Error())
			a.closer.Close()
		}
	}()

	// Given condition: support graceful shutdown.
	a.closer.Add(func() error {
		log.Printf("http: setting graceful timeout: %.2fs\n", a.config.GracefulTimeout.Seconds())
		ctx, cancel := context.WithTimeout(context.Background(), a.config.GracefulTimeout)
		defer cancel()

		log.Printf("http: awaiting traffic to stop: %.2fs\n", a.config.GracefulDelay.Seconds())
		time.Sleep(a.config.GracefulDelay)

		log.Println("http: shutting down: disabling keep-alive")
		srv.SetKeepAlivesEnabled(false)

		if err := srv.Shutdown(ctx); err != nil {
			return fmt.Errorf("http: shutting down: %w", err)
		}

		log.Println("http: gracefully stopped")
		return nil
	})

	// Waiting for signal to release all resources.
	a.closer.Wait()

	return nil
}
