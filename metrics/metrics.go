package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/broxgit/log"
)

const namespace = "viv"

// SetupMetrics is used to export the prometheus metrics.
func SetupMetrics(addr int) {
	address := strconv.FormatInt(int64(addr), 10)
	http.Handle("/metrics", promhttp.Handler())
	server := &http.Server{
		Addr:              ":" + address,
		ReadHeaderTimeout: 10 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil {
		log.ZError("Failed to start metrics server", log.Z("error", err))
	}
}

// DefaultBucket - default buckets for prometheus histograms.
var DefaultBucket = []float64{.001, .002, .004, .006, .008, .01, .02, .04, .06, .08, .1, .2, .4, .6, .8, 1, 2, 4, 6, 8, 10}

// HandleMessageTimerSeconds reports the amount of time in seconds it takes to handle a message. The amount of messages handled can also be obtained from this.
var HandleMessageTimerSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
	Namespace: namespace,
	Name:      "handle_message_seconds",
	Help:      "The amount of time in seconds it takes to handle a message",
	Buckets:   DefaultBucket,
})

// MessageOccurrence is a counter of how many times a specific message occurs.
var MessageOccurrence = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: namespace,
	Name:      "message_occurrence_total",
	Help:      "Count the occurrence of each message",
},
	[]string{"method"},
)

// HandleMessageErrors is a counter of the amount each error occurs.
var HandleMessageErrors = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: namespace,
	Name:      "handle_message_errors_total",
	Help:      "Total number of each error when attempting to handle a message",
},
	[]string{"reason"},
)
