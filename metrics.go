package service

import (
	"errors"
	"fmt"
	"github.com/broxgit/log"
	"github.com/broxgit/service/health"
	prometheusmetrics "github.com/broxgit/service/metrics"
	"net/http"
	"sync"
	"time"

	"github.com/felixge/fgprof"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var GoroutinesGauge = safeGauge(prometheus.GaugeOpts{
	Namespace: "viv",
	Name:      "num_goroutine_handlers",
	Help:      "Number of goroutines handling messages",
})

var DecodeMessageSeconds = safeHisto(prometheus.HistogramOpts{
	Namespace: "viv",
	Name:      "decode_message_seconds",
	Help:      "How long it takes to decode a message in seconds",
	Buckets:   prometheusmetrics.DefaultBucket,
})

var ProcessTimeSeconds = safeHisto(prometheus.HistogramOpts{
	Namespace: "viv",
	Name:      "process_time_seconds",
	Help:      "How long it takes to process a message. This includes decoding, handling, and acking the message",
	Buckets:   prometheusmetrics.DefaultBucket,
})

// OnCompleteMessageTimerSeconds reports the amount of time in seconds it takes to process the onComplete delegate for a message
var OnCompleteMessageTimerSeconds = safeHisto(prometheus.HistogramOpts{
	Namespace: "viv",
	Name:      "oncomplete_message_seconds",
	Help:      "The amount of time in seconds it takes to process the OnComplete delegate",
	Buckets:   prometheusmetrics.DefaultBucket,
})

var WorkerIdleTimeSeconds = safeHisto(prometheus.HistogramOpts{
	Namespace: "viv",
	Name:      "worker_idle_time_seconds",
	Help:      "Amount of time it takes for a worker to get a message",
	Buckets:   prometheusmetrics.DefaultBucket,
})

// startHTTPServer starts any http servers on ports defined in the configuration.
func startHTTPServer(c Config) {
	if err := prometheus.DefaultRegisterer.Register(GoroutinesGauge); err != nil {
		are := &prometheus.AlreadyRegisteredError{}
		if errors.As(err, are) {
			// A counter for that metric has been registered before.
			// Use the old counter from now on.
			if GoroutinesGauge != are.ExistingCollector.(prometheus.Gauge) {
				GoroutinesGauge = are.ExistingCollector.(prometheus.Gauge)
			}
		} else {
			log.ZError("Failed to register num_goroutine_handlers", log.Z("err", err))
		}
	}

	if c.HealthCheckServerDisabled {
		return
	}
	if c.PprofPort == 0 {
		c.PprofPort = 2112
	}
	if c.HealthCheckPort == 0 {
		c.HealthCheckPort = 2112
	}
	if c.PrometheusPort == 0 {
		c.PrometheusPort = 2112
	}

	mux := health.HealthChecksMux(c.Pingers)

	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/debug/fgprof", fgprof.Handler())

	servers := map[uint32]*http.Server{}
	// if any of these ports are the same, we just create extra objects that are immediately garbage collected
	// easier than a bunch of if checks
	for _, port := range []uint32{c.HealthCheckPort, c.PrometheusPort, c.PprofPort} {
		servers[port] = &http.Server{
			Addr:              fmt.Sprintf(":%d", port),
			Handler:           mux,
			ReadHeaderTimeout: 3 * time.Second, //nolint:mnd // recommended header timeout to prevent slowloris attack
		}
	}

	wg := &sync.WaitGroup{}
	wg.Add(len(servers))
	for port, server := range servers {
		go func(port uint32, server *http.Server) {
			defer wg.Done()
			err := server.ListenAndServe()
			if err != nil {
				log.ZError("Failed to start http server",
					log.Z("port", port),
					log.Z("error", fmt.Sprintf("%+v", err)))
			}
		}(port, server)
	}
	wg.Wait()
}

// safeGauge ensures that a Prometheus Gauge is registered safely.
// If the Gauge is already registered, it retrieves the existing one instead of causing a conflict.
//
// Returns a Prometheus Gauge that is either newly registered or safely reused.
func safeGauge(opts prometheus.GaugeOpts) prometheus.Gauge { //nolint:ireturn // blame prometheus
	ret := prometheus.NewGauge(opts)
	if err := prometheus.Register(ret); err != nil {
		already := &prometheus.AlreadyRegisteredError{}
		// If registration error is not due to existing metric, panic.
		if !errors.As(err, already) {
			panic(err)
		}
		var ok bool
		// Attempt to reuse the existing registered Gauge.
		if ret, ok = already.ExistingCollector.(prometheus.Gauge); !ok {
			panic("unable to reuse existing Gauge")
		}
	}
	return ret
}

// safeHisto ensures that a Prometheus Histogram is registered safely.
// If the Histogram is already registered, it retrieves the existing one instead of causing a conflict.
//
// Returns a Prometheus Histogram that is either newly registered or safely reused.
func safeHisto(opts prometheus.HistogramOpts) prometheus.Histogram {
	ret := prometheus.NewHistogram(opts)
	if err := prometheus.Register(ret); err != nil {
		already := &prometheus.AlreadyRegisteredError{}
		// If registration error is not due to existing metric, panic.
		if !errors.As(err, already) {
			panic(err)
		}
		var ok bool
		// Attempt to reuse the existing registered Histogram.
		if ret, ok = already.ExistingCollector.(prometheus.Histogram); !ok {
			panic("unable to reuse existing Histogram")
		}
	}
	return ret
}
