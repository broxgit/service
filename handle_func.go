package service

import (
	"context"
)

// HandleFunc is a function type that satisfies a Handler interface
type HandleFunc[T any] func(context.Context, T) error

// NewHandleFunc creates a new Handler[T] implementation using the provided HandleFunc.
// This allows simple function-based handlers to be used where a full Handler interface is required.
func NewHandleFunc[T any](lambda HandleFunc[T]) Handler[T] {
	return &lambdaHandler[T]{l: lambda}
}

// lambdaHandler is a simple wrapper that adapts a HandleFunc into a Handler[T] interface.
type lambdaHandler[T any] struct {
	l HandleFunc[T]
}

// Handle satisfies the Handler interface
func (d *lambdaHandler[T]) Handle(ctx context.Context, message T) error {
	return d.l(ctx, message)
}
