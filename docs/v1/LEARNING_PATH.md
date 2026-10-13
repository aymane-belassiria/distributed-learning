# v1 · The local slice (start here)

**Goal of v1:** get the four HR services running **end to end on your machine**,
with the smallest possible config at every step. You finish v1 with a working
distributed system — no Kubernetes, no Terraform, no cloud.

> **The v1 rule: minimum config, one concept at a time.**
> Add a dependency to `docker-compose.yml` only when the phase you're on needs
> it. Don't copy the "full" stack. Each phase below says exactly what to add and
> what to defer.

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

Ports: Postgres **55432**, Redis **6379**, Redpanda **9092**, services
**50051/50052/50053**, gateway **8080**.

---

## Checklist

- [ ] v1.0 — Minimal infra (Postgres + Redis)
- [ ] v1.1 — Contracts & sqlc (employee first)
- [ ] v1.2 — Employee service (gRPC + Postgres)
- [ ] v1.3 — Gateway + cache-aside
- [ ] v1.4 — Events: Redpanda + transactional outbox
- [ ] v1.5 — Leave service: validation + approvals ← the centerpiece
- [ ] v1.6 — Basic logging (structured + request id)
- [ ] v1.7 — Reporting read model (consumer + Redis)
- [ ] v1.8 — Run the whole slice end to end

---

## v1.0 · Minimal infra (Postgres + Redis)

**Goal:** two containers, nothing else.

**Add to `docker-compose.yml` — only this:**

```yaml
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: hr
      POSTGRES_PASSWORD: hr
      POSTGRES_DB: postgres
    ports: ["55432:5432"]
    volumes: ["pg_data:/var/lib/postgresql/data"]

  redis:
    image: redis:7-alpine
    ports: ["6379:6379"]

volumes:
  pg_data:
```

**Know it works**

```bash
docker compose up -d
docker compose ps
docker compose exec postgres psql -U hr -d postgres -c '\l'   # lists databases
docker compose exec redis redis-cli ping                       # PONG
```

**Deferred to v1.4:** Redpanda. **Deferred to v2:** LGTM observability stack,
Redis auth, healthchecks, `.env`, the three-database init script.

---

## v1.1 · Contracts & sqlc (employee first)

**Goal:** learn how `.proto` and SQL become typed Go — for **one** service.

**Create**
- `go mod init github.com/aymane-belassiria/hr-platform`
- `proto/hr/v1/employee.proto` — just `EmployeeService` with
  `CreateEmployee`, `GetEmployee`, `ListEmployees`.
- `buf.yaml` + `buf.gen.yaml` → `buf lint && buf generate` → look in `gen/`.
- `sqlc.yaml` with **one** block (database `employee`).
- `services/employee/db/migrations/0001_init.sql` (departments + employees) and
  `services/employee/db/queries/employees.sql`.
- `sqlc generate` → read the generated `db` package.

**Know it works:** `go build ./...` compiles and you can explain every override
in `sqlc.yaml` (e.g. `timestamptz` → `time.Time`).

**Defer:** the `leave`/`reporting` protos, their sqlc blocks and migrations.

**Guide:** [../v2/00-architecture.md](../v2/00-architecture.md) (data model),
[../v2/02-local-dev.md](../v2/02-local-dev.md) (generate/run).

---

## v1.2 · Employee service (gRPC + Postgres)

**Goal:** the first running service.

**Create** `services/employee/...` implementing `EmployeeService`: create (insert
employee), get, list. Add `cmd/server/main.go`. Run migrations from an embedded
FS on startup.

**Write tests:** employee code normalization; an integration test (Testcontainers)
for create + get.

**Prove it**

```bash
go run ./services/employee/cmd/server        # :50051
grpcurl -plaintext -d '{"first_name":"Amine","last_name":"Idrissi","email":"a@x.ma"}' \
  localhost:50051 hr.v1.EmployeeService/CreateEmployee
```

**Defer:** the outbox row (v1.4), the circuit breaker (v2), tracing (v2).

---

## v1.3 · Gateway + cache-aside

**Goal:** the REST edge with a real cache.

**Create** `services/gateway/` with a few routes:
`GET/POST /api/v1/employees`, `GET /api/v1/employees/{id}`, plus `/healthz`.
Cache the list in Redis; set an `X-Cache: HIT|MISS` header.

**Write tests:** the cache-aside helper (miss loads then stores; hit skips load).

**Prove it**

```bash
curl -sD- -o/dev/null localhost:8080/api/v1/employees | grep -i x-cache   # MISS
curl -sD- -o/dev/null localhost:8080/api/v1/employees | grep -i x-cache   # HIT
```

**Defer:** circuit breaker (v2), CORS/React (v2).

---

## v1.4 · Events: Redpanda + transactional outbox

**Goal:** publish reliably. This is where you add the third container.

**Add** Redpanda to `docker-compose.yml` (minimal):

```yaml
  redpanda:
    image: redpandadata/redpanda:v24.3.1
    command:
      - redpanda
      - start
      - --overprovisioned
      - --smp=1
      - --memory=1G
      - --check=false
      - --kafka-addr=PLAINTEXT://0.0.0.0:29092,OUTSIDE://0.0.0.0:9092
      - --advertise-kafka-addr=PLAINTEXT://redpanda:29092,OUTSIDE://localhost:9092
    ports: ["9092:9092"]
```

**Create**
- an `outbox` table; `CreateEmployee` writes the employee **and** an
  `employee.created` outbox row in **one transaction**.
- a relay that polls, publishes to Kafka, marks published.
- a thin `pkg/broker` producer.

**Prove it** (the outage test)

```bash
docker compose stop redpanda
curl -s -XPOST localhost:8080/api/v1/employees -H 'Content-Type: application/json' -d '{...}'
docker compose exec postgres psql -U hr -d employee \
  -c 'select count(*) from outbox where published_at is null;'   # grows
docker compose start redpanda                                    # relay drains it
```

**Defer:** consumer groups / idempotency (v1.7, v2.3).

---

## v1.5 · Leave service: validation + approvals ← the centerpiece

**Goal:** the real business logic.

**Add** the `leave` proto, its sqlc block, and a `leave` database (add an
`init.sql` or create the DB manually). Implement `LeaveService`:
- `workingDays(start, end, holidays)` — skip weekends/holidays.
- `ValidateLeaveRequest` — reject on: inactive/missing employee, bad dates, zero
  working days, overlap, insufficient balance, short notice, over max consecutive,
  unknown leave type. Return **all** reasons.
- `SubmitLeaveRequest` — validate → insert `requested` + outbox `leave.requested`.
- `DecideLeaveRequest` — approve/reject in a transaction; update the balance;
  guard against deciding twice.
- The service calls `employee.GetEmployee` over gRPC.

**Write tests (table-driven — the fun part):** one case per rule; plus a
"decide twice" test.

**Prove it**

```bash
# valid
curl -s -XPOST localhost:8080/api/v1/leave-requests -H 'Content-Type: application/json' \
  -d '{"employee_id":"<uuid>","leave_type":"annual","start_date":"2026-07-01","end_date":"2026-07-10"}'
# overlapping -> 4xx with machine-readable reasons
curl -s -XPOST localhost:8080/api/v1/leave-requests -d '{...overlapping...}'
```

**Defer:** the `btree_gist` constraint and isolation levels (v2.2), retries /
circuit breaker (v2.1).

---

## v1.6 · Basic logging (structured + request id)

**Goal:** logs you can actually follow across services.

**Create** `pkg/logger` — a `slog` JSON handler that adds a `service` field and a
per-request id (and, if you already carry a context value, a trace id). Wire it
into every service's main.

**Prove it:** make a leave request and follow the same request id through the
`gateway` → `leave` → `employee` log lines.

**Defer:** full OpenTelemetry (spans, metrics, exporters) → **v2.0**.

---

## v1.7 · Reporting read model (consumer + Redis)

**Goal:** build a read model from events.

**Add** the `reporting` service with a Kafka consumer (consumer group) handling
`leave.requested`/`leave.decided`/`employee.created`, incrementing Redis counters
(pending per department, days used per month). Expose
`GET /api/v1/reports/departments`.

**Prove it:** submit + approve and watch the summary change; replay the topic and
confirm no double counting.

**Defer:** durable projection, lag dashboards (v2.3, v2.0).

---

## v1.8 · Run the whole slice end to end

**Goal:** prove the four services work together.

- Start infra and all four services.
- Script one flow: create employee → cache MISS/HIT → submit leave (valid +
  overlapping) → approve → read the report.
- Write a short `docs/v1/NOTES.md` of what you learned and what still feels
  shaky.

When this passes, **v1 is done.** Next: [v2](../v2/LEARNING_PATH.md).

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
