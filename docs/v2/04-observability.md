# 04 · Observability

Three signals, one pipeline: services emit **traces**, **metrics** and **logs**;
an OpenTelemetry Collector fans them out to Tempo, Prometheus and Loki; Grafana is
the single pane of glass.

Locally this is the single `grafana/otel-lgtm` container. In production you would
run the collector, Tempo, Prometheus and Loki as separate deployments.

## How the services are instrumented

`pkg/otelx` builds a TracerProvider (OTLP gRPC exporter), a MeterProvider
(periodic OTLP reader), and sets W3C `TraceContext`+`Baggage` propagation.

Everything else is automatic:
- **HTTP**: `otelhttp.NewHandler` (gateway) → server span + `http.server.request.duration`.
- **gRPC**: `otelgrpc.NewServerHandler()` / `NewClientHandler()` (in `pkg/grpcx`) →
  client/server spans + `rpc.*` metrics.
- **Postgres**: pgx emits `db_client_*` metrics.
- **Redis**: `redisotel.InstrumentTracing/Metrics` in `pkg/cache`.
- **Kafka**: `pkg/broker` injects `traceparent` into record headers on produce
  and extracts it on consume, so a leave submission is traceable across the async hop.

## Logs are correlated, not separate

`pkg/logger` wraps `slog` and adds `trace_id`/`span_id` from the active span:

```json
{"time":"...","level":"INFO","msg":"http request","service":"gateway",
 "method":"POST","path":"/api/v1/leave-requests","status":201,
 "trace_id":"...","span_id":"..."}
```

Copy a `trace_id` into Grafana → Tempo to jump from a log line to its trace.

## Seeing it in Grafana (http://localhost:3300)

### Traces (Tempo → Explore)
Query `{}`, pick a trace. For a leave submission you should see:
`POST /api/v1/leave-requests` → `hr.v1.LeaveService/SubmitLeaveRequest` →
`hr.v1.EmployeeService/GetEmployee` → `db.statement` (balance/overlap queries) →
`kafka.publish leave.requested`.

### Metrics (Prometheus → Explore)
```promql
# gateway request rate by status
sum by (http_response_status_code) (rate(http_server_request_duration_seconds_count[1m]))

# p99 latency for the leave service
histogram_quantile(0.99, sum by (le) (rate(rpc_server_call_duration_seconds_bucket[5m])))

# gRPC error ratio (e.g. validation rejections)
sum(rate(rpc_server_call_duration_seconds_count{status_code!="0"}[5m]))
```

### Logs (Loki)
The local LGTM container does not ingest stdout by default. Use `kubectl logs` /
`docker compose logs` for structured JSON with trace IDs (they still correlate to
Tempo by `trace_id`), or add Grafana Alloy/Promtail to ship stdout to Loki.

## Adding your own span

```go
ctx, span := otel.Tracer("hr/leave").Start(ctx, "leave.validate")
defer span.End()
span.SetAttributes(attribute.String("leave.type", req.LeaveType),
                   attribute.Int("leave.working_days", days))
```

## Adding a custom metric

```go
meter := otel.GetMeterProvider().Meter("hr/leave")
rejected, _ := meter.Int64Counter("leave_requests_rejected_total")
rejected.Add(ctx, 1, metric.WithAttributes(attribute.String("rule", "overlap")))
```

A `leave_requests_rejected_total{rule=...}` counter is a great exercise — it turns
your validation rules into a dashboard.

## Verifying the pipeline (no UI)

```bash
curl -s 'http://localhost:9099/api/v1/query?query=otelcol_receiver_accepted_spans_total'
curl -s 'http://localhost:9099/api/v1/query?query=otelcol_receiver_accepted_metric_points_total'
curl -s "http://localhost:3201/api/search?q=%7B%7D&limit=5"   # Tempo search
```

> Metric export uses a 60s periodic reader, so app metrics can take up to a minute
> to appear after a service starts.

## Dashboards to build

- **RED** per service (rate, errors, duration).
- **Kafka lag** for the `reporting` consumer group (`rpk group describe reporting`).
- **Validation rejections** by rule (the counter above).
- **Approvals backlog**: pending `leave.requested` minus `leave.decided`.
- **Breaker state** changes exported as a metric.
