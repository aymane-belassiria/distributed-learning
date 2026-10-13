# 08 · Distributed patterns, explained with HR examples

Each pattern: what it is, where it lives in the HR platform, and an exercise that
makes it click. There is no answer key — the exercise is the spec.

> The data-layer patterns (indexes, isolation, replication, sharding, 2PC vs
> Sagas, distributed locks) get the full treatment in
> [09-databases.md](09-databases.md). What follows is the system-level view.

## 1. Database-per-service

**What:** each service owns its schema and is the only writer; no cross-service
joins.

**Here:** one Postgres server, three databases (`employee`, `leave`, `reporting`),
three migration directories, three `sqlc` blocks. `leave` never touches the
`employee` tables — it calls the employee gRPC API.

**Exercise:** add a `nickname` column to `employees` and confirm `leave` is
unaffected.

## 2. gRPC as the internal contract

**What:** typed, low-overhead RPC; protobuf is the source of truth.

**Here:** `proto/hr/v1/{employee,leave,reporting}.proto` → `gen/go/hr/v1` via
`buf generate`. `pkg/grpcx` adds tracing, logging, recovery, health and reflection
to every server.

**Exercise:** add a `SearchEmployees` RPC, regenerate, implement it, call it with
`grpcurl`.

## 3. Transactional outbox

**What:** publish reliably by writing the event into an `outbox` table in the
**same transaction** as the state change; a relay publishes afterwards. Avoids the
dual-write problem.

**Here:** `LeaveService.SubmitLeaveRequest` writes the request row and a
`leave.requested` outbox row in one tx; a relay publishes it. Same for
`DecideLeaveRequest` (`leave.decided`) and `employee.created`.

**Exercise:** stop Redpanda, submit leave requests, watch
`select count(*) from outbox where published_at is null` grow, restart Redpanda,
watch the relay drain the backlog.

## 4. At-least-once & idempotency

**What:** Kafka delivers at-least-once; consumers must tolerate redelivery.

**Here:** the reporting consumer commits offsets only after a successful
projection update; on error it does not commit, so the record replays. Malformed
keys are logged and committed (poison-message handling). Approvals are guarded by
a **status check** so a replayed `leave.decided` cannot double-count balance.

**Exercise:** apply the same `leave.approved` event twice and assert the Redis
counter increments once (use an event id or a status guard). Then do the same with
the DynamoDB idempotency table from v2.5.

## 5. Cache-aside (distributed cache)

**What:** read the cache; on a miss, load from the source and populate; invalidate
on write.

**Here:** the gateway caches the employee directory and department summaries in
Redis. Writes bust `hr:employees:` with SCAN+DEL. `X-Cache: HIT|MISS` tells you
which path ran.

**Exercise:** set the TTL to 1s and watch hits become misses; add a cache-hit-ratio
metric.

## 6. Circuit breaker

**What:** stop calling a failing dependency so you fail fast and let it recover.
Closed → open (after N failures) → half-open (probe) → closed.

**Here:** `pkg/breaker` wraps `sony/gobreaker`; the gateway uses a breaker per
downstream call, and `leave` uses one for its calls to `employee`. Open breakers
return `503` immediately.

**Exercise:** stop `employee`, fire 10 leave submissions, watch the breaker in
`leave` open; restart `employee` and watch the half-open probe recover it.

## 7. Event-driven read/write split (CQRS-lite)

**What:** serve hot reads from a fast store; build it from events.

**Here:** `reporting` consumes `leave.requested`/`leave.decided`/`employee.created`
and maintains Redis counters (pending per department, leave days per month). The
write path (`leave`) never computes reports.

**Exercise:** scale `reporting` to 2 replicas and watch Kafka rebalance the
partitions; then replay the topic and confirm no double counting.

## 8. Distributed tracing with context propagation

**What:** one trace follows a request across process and protocol boundaries.

**Here:** HTTP (`otelhttp`) → gRPC (`otelgrpc`, gateway → leave → employee) →
Kafka (the broker injects `traceparent`; the consumer extracts it). Logs carry the
same `trace_id`.

**Exercise:** submit a leave request and confirm the Tempo trace includes the
`employee.GetEmployee` span **and** the Kafka produce + consume spans.

## 9. Health, readiness and graceful shutdown

**What:** liveness says "restart me"; readiness says "send me traffic".

**Here:** gRPC services register the standard health service (TCP probe exposed);
the gateway has `/healthz` and `/readyz` (pings Redis). All services trap SIGTERM
and stop gracefully.

**Exercise:** `kubectl delete pod` a gateway replica while load-testing and watch
zero dropped requests.

## 10. Concurrency & correctness on approval

**What:** two managers hitting "approve" at the same time must not both win.

**Here:** `DecideLeaveRequest` runs in a transaction and only updates a request
that is currently `requested`; the second update affects zero rows and returns a
conflict. Balance is adjusted in the same transaction.

**Exercise:** fire two approvals concurrently in a test (goroutines) and assert
exactly one succeeds and the balance is adjusted once.

## 11. Configuration as twelve-factor

**What:** config from the environment; the binary is environment-agnostic.

**Here:** each service has a tiny `internal/config` reading env with defaults; the
same image runs on your laptop and in the cluster, differing only in
ConfigMap/Secret values.

**Exercise:** run the gateway with `LOG_LEVEL=debug CACHE_TTL=5s` and observe the
change without rebuilding.
