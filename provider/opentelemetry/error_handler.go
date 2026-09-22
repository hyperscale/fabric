package otel

import (
	"context"
	"log/slog"
	"sync"

	otelpkg "go.opentelemetry.io/otel"
)

var installErrorHandlerOnce sync.Once

// errorHandler reports errors the OpenTelemetry SDK cannot return to a caller
// (failed exports, dropped spans, ...). Without it the SDK falls back to
// log.Print, which slog.SetDefault bridges at INFO: an export failure then
// reads as an informational line and never trips an error-level alert.
type errorHandler struct {
	// logger is resolved on every call rather than captured once, so the
	// handler follows the slog.SetDefault done by the logger provider even when
	// it is installed first.
	logger func() *slog.Logger
}

var _ otelpkg.ErrorHandler = (*errorHandler)(nil)

func newErrorHandler(logger func() *slog.Logger) *errorHandler {
	return &errorHandler{logger: logger}
}

func (h *errorHandler) Handle(err error) {
	if err == nil {
		return
	}

	h.logger().LogAttrs(context.Background(), slog.LevelError, "opentelemetry error", slog.Any("error", err))
}

// installErrorHandler registers the error handler globally, once per process.
func installErrorHandler() {
	installErrorHandlerOnce.Do(func() {
		otelpkg.SetErrorHandler(newErrorHandler(slog.Default))
	})
}
