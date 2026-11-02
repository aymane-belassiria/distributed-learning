# v1 · The local slice (start here)

**Goal of v1:** get the four HR services running **end to end on your machine**,
with the smallest possible config at every step. You finish v1 with a working
distributed system — no Kubernetes, no Terraform, no cloud.

> **The v1 rule: minimum config, one concept at a time.**
> Add a dependency to `docker-compose.yml` only when the phase you're on needs
> it. Don't copy the "full" stack. Each ticket below says exactly what to add and
> what to defer.

**Session size:** every ticket is scoped to **one sitting (~1 hour) after work**.
Each one ends on a green checkpoint you can see with your own eyes — if a ticket
starts to sprawl, stop at the last checkpoint and pick up the next letter. You
never need to leave the repo in a broken state between sittings.

Once v1 works, go to [v2](../v2/LEARNING_PATH.md) for the ops + deep track and
the final assembled project.

---

## What "done" looks like

```
browser/curl → gateway (:8080) → leave (:50052) → employee (:50051)
                     │                  │
                     └── Redis cache    └── outbox → Redpanda (:9092) → reporting (:50053)
```

- `employee` — sqlc + Postgres, gRPC CRUD.
- `leave` — the validation + approval logic; calls `employee`.
- `reporting` — consumes events, Redis counters.
- `gateway` — REST BFF, cache-aside.

Ports: Postgres **55432** (you actually used **54322** — stay consistent with
your own compose), Redis **6379**, Redpanda **9092**, services
**50051/50052/50053**, gateway **8080**.

---

## Checklist

- [ ] **v1.0** — Minimal infra (Postgres + Redis)
- [ ] **v1.1** — Contracts & sqlc
  - [ ] v1.1a — proto + buf generate
  - [ ] v1.1b — sqlc config + schema + queries
- [ ] **v1.2** — Employee service
  - [ ] v1.2a — gRPC skeleton boots
  - [ ] v1.2b — implement Create / Get / List
  - [ ] v1.2c — unit tests
- [ ] **v1.3** — Gateway + cache-aside
  - [ ] v1.3a — REST gateway over gRPC
  - [ ] v1.3b — Redis cache-aside + `X-Cache`
  - [ ] v1.3c — cache-aside test
- [ ] **v1.4** — Events: Redpanda + outbox
  - [ ] v1.4a — Redpanda + producer
  - [ ] v1.4b — outbox table + transactional write
  - [ ] v1.4c — relay
  - [ ] v1.4d — outage drill
- [ ] **v1.5** — Leave service ← the centerpiece
  - [ ] v1.5a — leave proto + sqlc + DB
  - [ ] v1.5b — `workingDays` + tests
  - [ ] v1.5c — `ValidateLeaveRequest`
  - [ ] v1.5d — `SubmitLeaveRequest`
  - [ ] v1.5e — `DecideLeaveRequest`
  - [ ] v1.5f — call `employee` over gRPC
- [ ] **v1.6** — Basic logging
  - [ ] v1.6a — structured `slog` logger
  - [ ] v1.6b — request id across services
- [ ] **v1.7** — Reporting read model
  - [ ] v1.7a — Kafka consumer
  - [ ] v1.7b — Redis counters + endpoint
  - [ ] v1.7c — replay drill
- [ ] **v1.8** — Run the whole slice
  - [ ] v1.8a — end-to-end script
  - [ ] v1.8b — `NOTES.md` review

---

## v1.0 · Minimal infra (Postgres + Redis) · ⏱ ~1h

**Build:** `docker-compose.yaml` with just `postgres:16-alpine` and
`redis:7-alpine`, plus a named volume. Use the **mapping** form for env vars
(`POSTGRES_USER: hr`), not `- POSTGRES_USER = hr`.

**Done when:**
```bash
docker compose up -d && docker compose ps                     # both Up
docker compose exec postgres psql -U hr -d postgres -c '\l'   # lists DBs
docker compose exec redis redis-cli ping                      # PONG
```

**Defer:** Redpanda (v1.4a); LGTM stack, healthchecks, auth (v2).

---

## v1.1 · Contracts & sqlc

### v1.1a · proto + buf · ⏱ ~1h
**Build:** `go mod init github.com/aymane-belassiria/hr-platform`; `buf.yaml` +
`buf.gen.yaml`; `proto/hr/v1/employee.proto` with **one** service
(`EmployeeService`: `CreateEmployee`, `GetEmployee`, `ListEmployees`) and
`go_package` set to the module path.
**Done when:** `buf lint` clean, `buf generate` wrote `gen/go/hr/v1/*.pb.go`,
and `go build ./...` compiles the generated package.
**Defer:** leave/reporting protos (v1.5a).

### v1.1b · sqlc config + schema + queries · ⏱ ~1h
**Build:** `sqlc.yaml` with a **single** employee block;
`services/employee/db/migrations/0001_init.sql` (departments + employees);
`services/employee/db/queries/employees.sql` (Create / Get / List); run
`sqlc generate`.
**Done when:** the generated `db` package compiles, you can name every field of
`CreateEmployeeParams`, and there are **no stale generated files** (renaming a
query file means deleting the old `*.sql.go`).
**Defer:** leave/reporting sqlc blocks.

---

## v1.2 · Employee service

### v1.2a · gRPC skeleton boots · ⏱ ~1h
**Build:** create the `employee` database and apply the `-- +goose Up` half of
the migration by hand. Write `services/employee/cmd/server/main.go`: open a
`pgxpool`, `Ping`, `grpc.NewServer()`, register the service (methods may still
return `Unimplemented`), `reflection.Register`, listen on `:50051`, graceful
shutdown on SIGINT/SIGTERM.
**Done when:** the server logs "listening", and
`grpcurl -plaintext localhost:50051 list` shows `hr.v1.EmployeeService`.
**Defer:** embedded goose auto-migrations.

### v1.2b · Implement Create / Get / List · ⏱ ~1h
**Build:** `services/employee/internal/service/service.go` — embed
`UnimplementedEmployeeServiceServer`, implement the three RPCs against
`db.Queries`. Add small helpers: uuid ↔ string, empty-string → SQL `NULL` uuid,
default `employment_type`, generated `employee_code`. Map `pgx.ErrNoRows` →
`NotFound`, bad input → `InvalidArgument`.
**Done when:** `grpcurl` Create then Get/List return real rows; a bad uuid is
`NotFound`; a missing name/email is `InvalidArgument`.
**Defer:** outbox row (v1.4b), circuit breaker (v2.1), tracing (v2.0).

### v1.2c · Unit tests · ⏱ ~1h
**Build:** table tests for the pure helpers (`newEmployeeCode` format,
`defaultEmploymentType`).
**Done when:** `go test ./services/employee/...` is green.
**Defer:** Testcontainers integration test (v2.7c).

---

## v1.3 · Gateway + cache-aside

### v1.3a · REST gateway over gRPC · ⏱ ~1h
**Build:** `services/gateway/` with `/healthz`, `GET/POST /api/v1/employees`,
`GET /api/v1/employees/{id}`. Dial `employee` over gRPC; translate JSON ↔ proto.
**Done when:** `curl localhost:8080/api/v1/employees` returns the rows from
`employee`.

### v1.3b · Redis cache-aside · ⏱ ~1h
**Build:** connect the gateway to Redis; on list: read cache → miss = call
`employee` + store → set `X-Cache: HIT|MISS`.
**Done when:** the same curl twice shows `MISS` then `HIT`.

### v1.3c · cache-aside test · ⏱ ~1h
**Build:** a test for the helper (miss loads then stores; hit skips the load) with
a fake loader + in-memory/`miniredis`.
**Done when:** `go test ./services/gateway/...` green.
**Defer:** breaker, CORS/React (v2).

---

## v1.4 · Events: Redpanda + transactional outbox

### v1.4a · Redpanda + producer · ⏱ ~1h
**Build:** add the minimal `redpanda` service to compose (single broker, 1G,
`PLAINTEXT://redpanda:29092, OUTSIDE://localhost:9092`); a thin `pkg/broker`
producer.
**Done when:** a smoke program produces a message and you can read it back
(`rpk topic consume`).

### v1.4b · outbox table + transactional write · ⏱ ~1h
**Build:** add an `outbox` table; make `CreateEmployee` write the employee **and**
an `employee.created` outbox row in **one transaction**.
**Done when:** creating an employee leaves exactly one unpublished outbox row.

### v1.4c · relay · ⏱ ~1h
**Build:** a relay that polls unpublished rows, publishes to Kafka, marks
`published_at`.
**Done when:** after the relay runs, unpublished count is 0 and the topic has the
event.

### v1.4d · outage drill · ⏱ ~1h
**Build:** no code — run the drill.
**Done when:** stop Redpanda → create → unpublished count **grows** → start
Redpanda → the relay **drains** it. That's the whole point of the outbox.
**Defer:** consumer groups / idempotency (v1.7, v2.3).

---

## v1.5 · Leave service: validation + approvals ← the centerpiece

### v1.5a · leave proto + sqlc + DB · ⏱ ~1h
**Build:** `proto/hr/v1/leave.proto` (`LeaveService`); a second `sqlc.yaml` block;
create the `leave` database + migration (`leave_requests`, balances).
**Done when:** the generated leave `db` package compiles and the DB exists.

### v1.5b · `workingDays` + tests · ⏱ ~1h
**Build:** `workingDays(start, end, holidays)` — skip weekends/holidays.
**Done when:** a table test covers a weekend span and a holiday span.

### v1.5c · `ValidateLeaveRequest` · ⏱ ~1h
**Build:** reject on inactive/missing employee, bad dates, zero working days,
overlap, insufficient balance, short notice, over max consecutive, unknown type.
Return **all** reasons, not the first.
**Done when:** one table-driven case per rule is green.

### v1.5d · `SubmitLeaveRequest` · ⏱ ~1h
**Build:** validate → insert `requested` + outbox `leave.requested`, one tx.
**Done when:** a valid request persists and emits; an invalid one returns the
reason list.

### v1.5e · `DecideLeaveRequest` · ⏱ ~1h
**Build:** approve/reject in a transaction; update the balance; guard against
deciding twice.
**Done when:** deciding the same request twice returns a conflict (not a double
deduction).

### v1.5f · call `employee` over gRPC · ⏱ ~1h
**Build:** dial `employee` from `leave`; validate against the real employee +
department.
**Done when:** a request for a non-existent/terminated employee is rejected via
the gRPC lookup.
**Defer:** `btree_gist` + isolation levels (v2.2), retries/breaker (v2.1).

---

## v1.6 · Basic logging

### v1.6a · structured logger · ⏱ ~1h
**Build:** `pkg/logger` — an `slog` JSON handler that adds a `service` field; wire
it into every `main`.
**Done when:** every service's output is JSON with `"service":"..."`.

### v1.6b · request id across services · ⏱ ~1h
**Build:** middleware that generates/propagates a request id (gRPC metadata +
context) and logs it.
**Done when:** one leave request shows the **same** id in gateway → leave →
employee log lines.
**Defer:** OpenTelemetry spans/metrics (v2.0).

---

## v1.7 · Reporting read model

### v1.7a · Kafka consumer · ⏱ ~1h
**Build:** the `reporting` service with a consumer group subscribed to
`leave.requested` / `leave.decided` / `employee.created`.
**Done when:** events are consumed and logged.

### v1.7b · Redis counters + endpoint · ⏱ ~1h
**Build:** increment Redis counters (pending per dept, days used per month) and
expose `GET /api/v1/reports/departments`.
**Done when:** submit + approve visibly changes the summary.

### v1.7c · replay drill · ⏱ ~1h
**Build:** no code — replay the topic from the start.
**Done when:** either counts don't double (you built it idempotent) **or** you can
explain exactly why they do → that's the v2.3 ticket.
**Defer:** durable projection, lag dashboards (v2.3, v2.0).

---

## v1.8 · Run the whole slice end to end

### v1.8a · end-to-end script · ⏱ ~1h
**Build:** one script: create employee → cache MISS/HIT → submit leave (valid +
overlapping) → approve → read the report.
**Done when:** the script passes against a fresh `docker compose up -d`.

### v1.8b · NOTES.md · ⏱ ~1h
**Build:** `docs/v1/NOTES.md` — what you learned, what still feels shaky.
**Done when:** it's written. **v1 is done.** → [v2](../v2/LEARNING_PATH.md)

---

## What you deliberately did NOT do in v1

| Deferred | Goes to |
| -------- | ------- |
| OpenTelemetry traces/metrics, Grafana/Loki/Tempo | v2.0 |
| Circuit breaker, retries/backoff | v2.1 |
| indexes, `EXPLAIN`, isolation, replication, sharding | v2.2 |
| idempotency, at-least-once hardening, CQRS durability | v2.3 |
| Kubernetes | v2.4 |
| Terraform → Floci | v2.5 |
| Ansible | v2.6 |
| CI/CD | v2.7 |
| React UI | v2.8 |
| Final assembled project + hardening | v2.9 |
