package slogger

import (
	"context"
	"log/slog"

	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
)

type scopeContextKey struct{}

// ContextWithScope returns a context carrying an OpenTelemetry scope name.
// The handler installed by Setup adds it to log records as "otel.scope.name".
// A non-empty name overrides an inherited scope; an empty name leaves ctx unchanged.
func ContextWithScope(ctx context.Context, name string) context.Context {
	if name == "" {
		return ctx
	}

	return context.WithValue(ctx, scopeContextKey{}, name)
}

type scopeHandler struct {
	next slog.Handler
}

func (h scopeHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h scopeHandler) Handle(ctx context.Context, record slog.Record) error {
	scope, ok := scopeFromContext(ctx)
	if !ok {
		return h.next.Handle(ctx, record)
	}

	record = record.Clone()
	record.AddAttrs(slog.String(string(semconv.OTelScopeNameKey), scope))

	return h.next.Handle(ctx, record)
}

func (h scopeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return scopeHandler{next: h.next.WithAttrs(attrs)}
}

func (h scopeHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}

	return scopeHandler{next: h.next.WithGroup(name)}
}

func scopeFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}

	scope, ok := ctx.Value(scopeContextKey{}).(string)

	return scope, ok
}
