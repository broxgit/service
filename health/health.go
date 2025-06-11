package health

import (
	"context"
	"net/http"
	"time"

	"github.com/broxgit/log"
)

type Pinger interface {
	Ping(ctx context.Context) error
}

// NamedPinger represents an interface for pinging an object as a health check with a given name.
type NamedPinger interface {
	Name() string
	Pinger
}

// PingerFunc helps convert a function without context parameter into a Pinger
type PingerFunc func() error

func (f PingerFunc) Ping(_ context.Context) error { return f() }

// NewNamedPingerFunc helps create named pingers when only a pinger is available
//
//	ex: service.Create(src, handler, service.WithPingers(NewPinger("Account System", accountsystemClient.Ping)))
func NewNamedPingerFunc(name string, p PingerFunc) NamedPinger {
	return &pingerWrapper{name: name, p: p}
}

func NewPinger(name string, p Pinger) NamedPinger {
	return &pingerWrapper{name: name, p: p}
}

type pingerWrapper struct {
	name string
	p    Pinger
}

func (p pingerWrapper) Ping(ctx context.Context) error { return p.p.Ping(ctx) }
func (p pingerWrapper) Name() string                   { return p.name }

type healthChecker struct {
	pingers map[string]Pinger
}

func (hc *healthChecker) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	for pingerName, pinger := range hc.pingers {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second) //nolint:mnd // default timeout
		if err := pinger.Ping(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			log.ZWarn("Health Check failed", log.Z("err", err), log.Z("pingerName", pingerName))
			cancel()
			return
		}
		cancel()
	}
	w.WriteHeader(http.StatusOK)
}

// HealthChecksMux returns a http.ServeMux with ready and live checks using the given pingers.
func HealthChecksMux(checks map[string]Pinger) *http.ServeMux { //nolint:revive // naming is fine
	// Use the DefaultServeMux here so we get the pprof paths
	m := http.DefaultServeMux
	m.Handle("/health/ready", &healthChecker{pingers: checks})
	m.HandleFunc("/health/live", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return m
}
