// package service provides interfaces and structures for processing messages from various sources.
// The primary functionality is centered around the `Create` function, which facilitates concurrent
// message processing by piping messages from sources to handlers.
//
// ## Key Features:
// - **Concurrent Processing:** Messages are processed in a multi-goroutine environment.
// - **Lifecycle Management:** Ensures all messages are fully processed before shutdown.
// - **Tracing & Metrics:** Integrated support for distributed tracing and Prometheus metrics.
package service

import (
	"context"
	"errors"
	"iter"
	_ "net/http/pprof" //nolint:gosec // intentionally expose pprof
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/broxgit/log"
	"github.com/broxgit/service/health"
	botel "github.com/broxgit/service/internal/otel"
	prometheusmetrics "github.com/broxgit/service/metrics"
)

// Tracker defines an interface for tracking the lifecycle of message processing.
type Tracker interface {
	// Context returns a context associated with the current message processing.
	Context() context.Context

	// OnComplete is called once the message handler has completed processing.
	// The provided error indicates whether an issue was encountered during processing.
	// This can be used for logging, metric collection, or any necessary cleanup.
	OnComplete(error)
}

// Source represents an interface as the entry point for messages that need processing.
type Source[T any] interface {
	Messages(WorkGroup) iter.Seq2[T, Tracker]
	health.NamedPinger
	OnComplete(error)
}

// Handler defines a component responsible for processing messages of type T.
type Handler[T any] interface {
	// Handle processes a message from a source of type T.
	//
	// This method does not need to respect the context's cancellation signal,
	// as messages are always given enough time to complete processing. If the
	// handler does respect the context and aborts execution, the message will
	// not be retried. Instead, it is recommended to always attempt processing
	// to completion, regardless of cancellation.
	//
	// If processing is interrupted, the message is guaranteed to be retried
	// in a future attempt. To ensure correctness, the handler's logic should
	// be idempotent.
	//
	// Can return an error if processing fails. The error is returned to the Source,
	// which determines how to handle it. Refer to the Source documentation
	// to understand the implications of returning an error.
	//
	// Some Source implementations may retry the message upon error, potentially
	// leading to an infinite loop if the error is non-retryable. Ensure that
	// errors are properly handled to avoid unintended reprocessing.
	Handle(context.Context, T) error
}

// Decoder provides a mechanism for transforming data between types.
type Decoder[From, To any] interface {
	// Decode converts a value of type `From` to type `To`.
	// Returns the transformed value and any error encountered during conversion.
	Decode(From) (To, error)
}

// Service is a generic component that continuously:
// 1. Fetches messages from a `Source`
// 2. Processes them using a `Handler`
// 3. Manages lifecycle events (graceful shutdown, telemetry, etc.)
type Service[T any] struct {
	// Source is the data ingress point for the service, responsible for providing messages.
	Source[T]

	// Handler is the processing component that handles each message.
	Handler[T]

	// conf holds the service configuration.
	conf Config

	// tracer is used for distributed tracing and telemetry.
	tracer trace.Tracer

	sync.Once
}

// Create initializes a `Service` instance.
// Once created, it should be started using `Start()`.
func Create[T any](src Source[T], handler Handler[T], opts ...Option) *Service[T] {
	conf := NewConfig(opts...)
	conf.Pingers[src.Name()] = src

	GoroutinesGauge.Set(float64(conf.GoRoutines))
	go startHTTPServer(*conf)

	return &Service[T]{
		Source:  src,
		Handler: handler,
		conf:    *conf,
	}
}

// stop ensures all messages are fully processed before termination.
// If shutdown takes too long, it forces termination after 30 seconds.
func (s *Service[_]) stop(work WorkGroup) {
	ctx, cancel := NewSigContext(context.Background())
	defer cancel() // Not used, but for completeness we always cancel contexts
	work.Cancel(WorkComplete)

	select {
	case <-work.Complete():
	case <-ctx.Done():
		log.ZWarn("Force shutting down after receiving double signals")
	case <-time.After(time.Second * 30):
		log.ZWarn("Force shutting down after 30s timeout") //nolint:mnd // timeout
	}
}

// Start begins the service execution and continues until the context is canceled.
// It Ensures all messages received before shutdown are fully processed.
// If a `Tracker` is associated with a message, its `OnComplete` method is guaranteed to be called.
func (s *Service[T]) Start(parent context.Context) error {
	ctx, cancel := NewSigContext(parent)
	defer cancel()
	defer func() {
		s.OnComplete(nil)
	}()

	work := NewWorkGroup(ctx)
	work.Add(s.conf.GoRoutines)

	type workerMsg struct {
		tracker Tracker
		msg     T
	}

	msgs := make(chan workerMsg, s.conf.GoRoutines*2)
	for range s.conf.GoRoutines {
		go func() {
			defer work.WorkerDone()
			for {
				timer := prometheus.NewTimer(WorkerIdleTimeSeconds)
				m, ok := <-msgs
				timer.ObserveDuration()
				if !ok {
					return
				}
				s.processMsg(m.msg, m.tracker)
			}
		}()
	}

	for msg, tracker := range s.Messages(work) {
		msgs <- workerMsg{tracker: tracker, msg: msg}
	}

	// signal shutdown handlers, continue to finish processing all fetched messages
	close(msgs)

	select {
	case <-ctx.Done(): // generally a sigint
		s.stop(work)
		return ctx.Err()
	case <-work.Complete():
		return work.Err()
	}
}

// processMsg handles the execution of a single message.
func (s *Service[T]) processMsg(msg T, tracker Tracker) {

	pTimer := prometheus.NewTimer(ProcessTimeSeconds)
	defer pTimer.ObserveDuration()
	ctx := context.Background()
	if tracker != nil {
		ctx = tracker.Context()
	}

	//ctx, span := s.getSpan(ctx)
	//defer span.End()
	handleTimer := prometheus.NewTimer(prometheusmetrics.HandleMessageTimerSeconds)
	err := s.Handle(ctx, msg)
	handleTimer.ObserveDuration()

	if tracker != nil {
		onCompleteTimer := prometheus.NewTimer(OnCompleteMessageTimerSeconds)
		tracker.OnComplete(err)
		onCompleteTimer.ObserveDuration()
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		log.ZError("Error handling message", log.Z("err", err))
		//span.SetStatus(codes.Error, err.Error())
	}
}

// getSpan initializes a new tracing span to track message processing.
// OpenTelemetry does not support span re-use across streaming gRPC requests.
// Therefore, each message starts a new trace for better observability.
//
// See: https://github.com/open-telemetry/opentelemetry-go-contrib/issues/436
func (s *Service[_]) getSpan(ctx context.Context) (context.Context, trace.Span) {
	s.Do(func() {
		botel.SetGlobalTracerProvider(ctx)
		s.tracer = otel.GetTracerProvider().Tracer("source.vivint.com/pl/service/v4/service")
	})

	ctx, span := s.tracer.Start(ctx, "handle",
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithNewRoot(),
	)
	span.SetStatus(codes.Ok, "")
	return ctx, span
}
