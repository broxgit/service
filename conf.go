package service

import (
	"github.com/broxgit/env"
	"github.com/broxgit/service/health"
)

// Default values for the Config struct
const defaultPort = 2112

var defaultGoRoutines = env.Int("GOROUTINES", 50)

// Config is the config struct for Service.
type Config struct {
	GoRoutines int

	// PprofPort defaults to 2112, but you can override if you want. (You dont...)
	PprofPort uint32

	// Pingers adds health check on these pingers
	Pingers map[string]health.Pinger

	// HealthCheckPort defaults to 2112, but you can override if you want. (You dont...)
	HealthCheckPort uint32

	// PrometheusPort defaults to 2112, but you can override it if you want. (You dont...)
	PrometheusPort uint32

	// HealthCheckServerDisabled will prevent starting the health check server if set.  Note,
	// the health check serve sets up handlers for liveness/readiness endpoints, metrics, and
	// pprof - all of which will be unavailable when this flag is set.
	HealthCheckServerDisabled bool
}

// Option represents a functional option for configuring the Config struct
type Option func(*Config)

// WithGoRoutines sets the number of goroutines
func WithGoRoutines(n int) Option {
	return func(c *Config) {
		c.GoRoutines = n
	}
}

// WithPprofPort sets the PprofPort
func WithPprofPort(port uint32) Option {
	return func(c *Config) {
		c.PprofPort = port
	}
}

// WithPingers sets the Pingers map
func WithPingers(pingers ...health.NamedPinger) Option {
	return func(c *Config) {
		for _, p := range pingers {
			c.Pingers[p.Name()] = p
		}
	}
}

// WithHealthCheckPort sets the HealthCheckPort
func WithHealthCheckPort(port uint32) Option {
	return func(c *Config) {
		c.HealthCheckPort = port
	}
}

// WithPrometheusPort sets the PrometheusPort
func WithPrometheusPort(port uint32) Option {
	return func(c *Config) {
		c.PrometheusPort = port
	}
}

// WithHealthCheckServerDisabled disables the health check server
func WithHealthCheckServerDisabled(disabled bool) Option {
	return func(c *Config) {
		c.HealthCheckServerDisabled = disabled
	}
}

// NewConfig creates a new Config instance with defaults and applies given options
func NewConfig(opts ...Option) *Config {
	cfg := &Config{
		GoRoutines:                defaultGoRoutines,
		PprofPort:                 defaultPort,
		HealthCheckPort:           defaultPort,
		PrometheusPort:            defaultPort,
		Pingers:                   map[string]health.Pinger{},
		HealthCheckServerDisabled: false,
	}

	for _, opt := range opts {
		opt(cfg)
	}

	return cfg
}
