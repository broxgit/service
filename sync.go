package service

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/broxgit/log"
)

// NewSigContext returns a context that cancels when receiving SIGINT or SIGTERM.
func NewSigContext(parent context.Context) (context.Context, context.CancelFunc) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(parent)

	go func() {
		select {
		case sig := <-ch:
			log.ZInfo("Received signal, canceling", log.Z("sig", sig))
		case <-ctx.Done():
		}
		cancel()
	}()

	return ctx, cancel
}

var (
	WorkComplete = errors.New("Work Complete")
)

// WorkGroup combines ideas of sync.WaitGroup and context.Context to provide both cancel and work complete signals.
type WorkGroup interface {
	Context() context.Context
	Cancel(cause error)
	Canceled() <-chan struct{}
	Err() error

	With(context.Context) (context.Context, context.CancelFunc)

	Add(int)
	WorkerDone()
	Complete() <-chan struct{}
}

// workGroup wraps sync.WaitGroup to give a context and a waitGroup functionality joined together.
type workGroup struct {
	ctx    context.Context
	cancel context.CancelCauseFunc

	wg       *sync.WaitGroup
	complete chan struct{}

	init sync.Once
}

// NewWorkGroup creates a waitgroup with the parent context.
func NewWorkGroup(parent context.Context) WorkGroup {
	ctx, cancel := context.WithCancelCause(parent)

	return &workGroup{
		wg: &sync.WaitGroup{},

		ctx:      ctx,
		cancel:   cancel,
		complete: make(chan struct{}),
	}
}

// setup creates a go routine listening for the waitgroup done event.
func (w *workGroup) setup() {
	go func() {
		w.wg.Wait()
		w.Cancel(WorkComplete) // guarantee that cancel happens before complete
		close(w.complete)
	}()
}

func (w *workGroup) Context() context.Context {
	return w.ctx
}

func (w *workGroup) Cancel(err error) {
	w.cancel(err)
}

func (w *workGroup) Err() error {
	err := context.Cause(w.ctx)
	if errors.Is(err, WorkComplete) {
		return nil
	}
	return err
}

// With creates work with the given context. The cancel func must be called to represent the work being complete.
func (w *workGroup) With(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		return w.ctx, func() { w.cancel(WorkComplete) }
	}

	w.Add(1)
	ctx, cancel := context.WithCancelCause(ctx)
	cancelWork := func() {
		w.WorkerDone()
		cancel(WorkComplete)
	}
	stop := context.AfterFunc(w.ctx, cancelWork)
	context.AfterFunc(ctx, func() { stop() })
	return ctx, cancelWork
}

// Add adds to the workGroup count. See sync.WaitGroup.Add.
func (w *workGroup) Add(n int) { w.wg.Add(n) }

// WorkerDone calls WaitGroup.Done. see sync.WaitGroup.Done.
func (w *workGroup) WorkerDone() { w.wg.Done() }

// Canceled returns a channel that closes after the context is Done.
func (w *workGroup) Canceled() <-chan struct{} { return w.ctx.Done() }

// Complete returns a channel that closes when the waitgroup is done.
func (w *workGroup) Complete() <-chan struct{} {
	w.init.Do(w.setup)
	return w.complete
}
