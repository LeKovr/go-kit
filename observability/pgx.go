package observability

import (
	"context"
	"strings"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PGXTracer returns instrumentation for pgx ConnConfig.Tracer, set before connecting.
// Service providers are used without OTel globals; SQL text and parameters are omitted.
func (svc Service) PGXTracer() *otelpgx.Tracer {
	return otelpgx.NewTracer(
		otelpgx.WithTracerProvider(svc.tracerProviderOrNoop()),
		otelpgx.WithMeterProvider(svc.meterProviderOrNoop()),
		otelpgx.WithDisableSQLStatementInAttributes(),
		otelpgx.WithQuerySpanNamePrefix(),
		otelpgx.WithSpanNameCtxFunc(pgxSpanName),
		otelpgx.WithMetricOperationName(pgxSpanName),
	)
}

// RecordPGXPoolStats registers pool metrics once per pool, owned by the caller.
// The Service meter provider takes precedence over opts; globals are not used.
func (svc Service) RecordPGXPoolStats(pool *pgxpool.Pool, opts ...otelpgx.StatsOption) error {
	options := make([]otelpgx.StatsOption, 0, len(opts)+1)
	options = append(options, opts...)
	options = append(options, otelpgx.WithStatsMeterProvider(svc.meterProviderOrNoop()))

	return otelpgx.RecordStats(pool, options...)
}

func pgxSpanName(ctx context.Context, sql string) string {
	name := otelpgx.SQLOperationName(ctx, sql)
	if i := strings.IndexFunc(name, func(r rune) bool { return r < 'A' || r > 'Z' }); i >= 0 {
		name = name[:i]
	}

	// Only SQL keywords may become names; upstream's first token can contain literals.
	switch name {
	case "ALTER", "ANALYZE", "BEGIN", "CALL", "COMMIT", "COPY", "CREATE",
		"DEALLOCATE", "DECLARE", "DELETE", "DISCARD", "DO", "DROP", "END",
		"EXECUTE", "EXPLAIN", "FETCH", "GRANT", "INSERT", "LISTEN", "LOCK",
		"MERGE", "MOVE", "NOTIFY", "PREPARE", "REFRESH", "REINDEX", "RELEASE",
		"RESET", "REVOKE", "ROLLBACK", "SAVEPOINT", "SELECT", "SET", "SHOW",
		"START", "TRUNCATE", "UNLISTEN", "UPDATE", "VACUUM", "VALUES", "WITH":
		return name
	default:
		return "UNKNOWN"
	}
}
