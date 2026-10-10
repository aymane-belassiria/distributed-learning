# LEARNING_PATH — build an HR platform yourself

A phase-by-phase workflow for building a **distributed HR / leave-management
system** from an empty folder. Each phase has a **goal**, the **concepts**, the
**files you create**, how you **know it works**, and a **prove-it** step where you
deliberately break the system to watch the pattern behave.

This folder starts empty — only this file and `docs/`. There is no code, config,
scaffolding, or generated code; you write all of it. There is deliberately **no
answer key**: the phases are the spec, and the tests you write are the contract.

> Rule: finish a phase (its checks pass) before starting the next.

---

## What you are building

An HR platform where HR staff manage employees and employees request leave that
gets **validated** (no overlaps, enough balance, working-days only) and
**approved** by a manager. It is small enough to reason about, but exercises the
same distributed machinery as a real system.

### Services

| Service     | Owns (its database)                         | Talks to                    | Port  |
| ----------- | ------------------------------------------- | --------------------------- | ----- |
| `employee`  | employees, departments                      | —                           | 50051 |
| `leave`     | leave_requests, leave_balances, holidays    | `employee` (gRPC)           | 50052 |
| `reporting` | leave projections, dept aggregates          | Kafka (consumer), Redis     | 50053 |
| `gateway`   | nothing (stateless BFF)                     | all three (gRPC), Redis     | 8080  |

### Data model (starting point — evolve it as you like)

```
departments   (id, code, name, created_at)
employees     (id, employee_code, first_name, last_name, email, department_id,
               position, manager_id, hire_date, employment_type, status, created_at)
leave_types   (code, name, default_days_per_year)            -- annual, sick, unpaid
holidays      (id, country, day, name)                       -- to skip public holidays
leave_requests(id, employee_id, leave_type, start_date, end_date, working_days,
               status, reason, requested_at, decided_at, decided_by, decision_note)
leave_balances(employee_id, leave_type, year, entitled_days, used_days)
outbox        (id, topic, key, payload, created_at, published_at)
```

### Events (Kafka)

- `employee.created` — when an employee is onboarded
- `leave.requested` — a validated request awaiting a decision
- `leave.decided`  — approved or rejected

### The interesting logic (the "validate vacation" part)

`leave.SubmitLeaveRequest` must reject a request when **any** of these hold:

1. employee does not exist or is not `active`
2. `start_date > end_date`, or the range is entirely in the past
3. leave type is unknown
4. the range spans zero **working days** (weekends and public holidays excluded)
5. working days exceed the maximum consecutive allowance (e.g. 30)
6. it overlaps an existing non-cancelled request for that employee
7. the employee lacks balance for that leave type (sick leave may be exempt)
8. the notice period is too short (annual leave needs ≥ N days' notice)

On success it persists the request as `requested` **and** writes `leave.requested`
to the outbox in the same transaction.

---

## Checklist

- [ ] Phase 0 — Orientation & setup
- [ ] Phase 1 — Contracts & code generation (buf, sqlc)
- [ ] Phase 2 — Employee service (sqlc + outbox write)
- [ ] Phase 3 — Observability: logging + OTel SDK
- [ ] Phase 4 — gRPC layer + circuit breaker
- [ ] Phase 5 — Gateway BFF + distributed cache
- [ ] Phase 6 — Kafka events + transactional outbox relay
- [ ] Phase 7 — Leave service: validation + approvals  ← the centerpiece
- [ ] Phase 8 — Reporting service (Redis counters + Kafka consumer)
- [ ] Phase 9 — Kubernetes with kind + Kustomize
- [ ] Phase 10 — Terraform → Floci (local AWS + EKS)
- [ ] Phase 11 — Ansible
- [ ] Phase 12 — CI/CD
- [ ] Phase 13 — Testing & hardening

---

## Phase 0 · Orientation & setup

**Goal:** turn an empty folder into a Go module with local infrastructure.

**Concepts:** database-per-service; gRPC internally, REST at the edge; OTLP to a
collector. Read [docs/00-architecture.md](docs/00-architecture.md).

**Create**
- `go mod init github.com/aymane/hr-platform`
- the layout:
  ```
  proto/hr/v1/                         # employee.proto, leave.proto, reporting.proto
  services/{employee,leave,reporting,gateway}/
  pkg/                                 # shared libs (logger, otelx, cache, breaker, broker, grpcx)
  deploy/  infra/  web/  scripts/
  docker-compose.yml  Makefile  .env.example
  ```
- `docker-compose.yml` with Postgres, Redis, Redpanda and an observability stack
  (`grafana/otel-lgtm` is the easy one). Non-conflicting host ports: Postgres
  **55432**, Redis 6379, Redpanda 9092, Grafana **3300**, OTLP 4317/4318,
  Prometheus **9099**, Loki **3101**, Tempo **3201** (5432/3000/9090 are taken on
  this host).
- a `Makefile`: `dev-up`, `dev-down`, `generate`, `test`,
  `run-employee`, `run-leave`, `run-reporting`, `run-gateway`.
- `git init && git add -A && git commit -m "phase 0"`.

**Know it works:** `docker compose ps` shows everything healthy and Grafana loads
at http://localhost:3300 (admin/admin).

**Prove it:** `docker compose down && docker compose up -d` brings it back with
data intact.

---

## Phase 1 · Contracts & code generation

**Goal:** know how protobuf and sqlc turn sources into typed Go.

**Concepts:** `buf` lint/generate; `sqlc` query analysis; SQL types → Go types.

**Create**
- `proto/hr/v1/employee.proto`, `leave.proto`, `reporting.proto`
  (package `hr.v1`; `go_package = github.com/aymane/hr-platform/gen/go/hr/v1;hrv1`).
  Define `EmployeeService`, `LeaveService`, `ReportingService` messages/RPCs.
- `buf.yaml` + `buf.gen.yaml` (protoc-gen-go + protoc-gen-go-grpc, `paths=source_relative`).
- `sqlc.yaml` with one block per database (`employee`, `leave`, `reporting`),
  `sql_package: pgx/v5`, overrides for `uuid` / `timestamptz` / `jsonb`.
- goose migrations + queries under `services/<svc>/db/{migrations,queries}`.

**Do:** `buf lint` → `buf generate` → inspect `gen/go/hr/v1/*.pb.go`;
`sqlc generate` → inspect the generated `db` package; add a query of your own and
read the generated `Params` struct.

**Know it works:** generated Go compiles (`go build ./...`) and you can explain
every override in `sqlc.yaml`.

**Prove it:** rename a field in a `.proto`, regenerate, watch the Go type change.

---

## Phase 2 · Employee service

**Goal:** the core writer: sqlc + Postgres + a transactional outbox row.

**Concepts:** pgx transactions; upsert with `ON CONFLICT`; code normalization;
mapping `pgx.ErrNoRows` → gRPC `NotFound`; writing the event in the same tx.

**Create:** `services/employee/...` implementing `EmployeeService`:
- `normalizeEmployeeCode` (e.g. `"Amine El Idrissi"` + a number → `EMP-00042` or
  `amine.el-idrissi`), `defaultEmploymentType` (empty → `full_time`).
- `CreateEmployee` — insert employee + `employee.created` outbox row in one tx.
- `GetEmployee`, `GetEmployeeByCode`, `ListEmployees` (filters, paging).
- wire migrations (`goose` with an embedded FS) and `cmd/server/main.go`.

**Write these tests (your spec)**
- `TestNormalizeEmployeeCode`, `TestDefaultEmploymentType`
- an integration test (Testcontainers Postgres) for create + list + get.

**Prove it**
```bash
make run-employee
grpcurl -plaintext -d '{"first_name":"Amine","last_name":"Idrissi","email":"a@x.ma","department_id":"<uuid>"}' \
  localhost:50051 hr.v1.EmployeeService/CreateEmployee
psql ... -c 'select employee_code,status from employees; select count(*) from outbox;'
```
You should see an employee row **and** an outbox row.

---

## Phase 3 · Observability: logging + OTel SDK

**Goal:** correlate logs with traces and set up the tracer/meter providers.

**Concepts:** slog handlers; OTel TracerProvider/MeterProvider; OTLP exporters;
W3C trace-context propagation.

**Create**
- `pkg/logger/logger.go` — a `slog.Handler` wrapper that adds `trace_id`/`span_id`
  from the active span.
- `pkg/otelx/otelx.go` — build the providers + OTLP exporters + propagator; return
  a shutdown function.

**Write this test:** `TestLoggerAddsTraceID` — assert a log line emitted inside a
span contains `trace_id` and `span_id`.

**Prove it:** run a service, make a request, copy the `trace_id` from the JSON log
line into Grafana → Tempo and find the trace.

---

## Phase 4 · gRPC layer + circuit breaker

**Goal:** a common gRPC server/client stack and a reusable breaker.

**Concepts:** unary interceptors; `otelgrpc` stats handlers; gRPC health +
reflection; the circuit-breaker state machine (closed/open/half-open).

**Create**
- `pkg/grpcx/grpcx.go` — `NewServer` (otel + logging + panic recovery + health +
  reflection) and `Dial` (insecure + otelgrpc + round-robin).
- `pkg/breaker/breaker.go` — generic `Execute`/`State` over `sony/gobreaker`.

**Write these tests:** `TestBreakerOpensAfterConsecutiveFailures`,
`TestBreakerPassesThroughWhenClosed`.

**Prove it:** stop the `employee` service and watch the `leave` service fail fast
instead of hanging.

---

## Phase 5 · Gateway BFF + distributed cache

**Goal:** the REST edge that fans out over breakers and caches hot reads.

**Concepts:** BFF; cache-aside; Redis SCAN+DEL invalidation; mapping gRPC status
codes to HTTP; protojson at the edge; CORS for the React app.

**Create**
- `pkg/cache/cache.go` — `New`, `SetJSON`, `GetJSON`, `Del`, `DelPrefix`, `GetOrLoad`.
- `services/gateway/internal/httpapi/server.go` — routes:
  `GET/POST /api/v1/employees`, `GET /api/v1/employees/{id}`,
  `POST /api/v1/leave-requests`, `GET /api/v1/leave-requests`,
  `POST /api/v1/leave-requests/{id}/decide`,
  `GET /api/v1/employees/{id}/balance`, `GET /api/v1/reports/departments`,
  plus `/healthz`, `/readyz`.

**Write these tests:** `TestGetOrLoadIsCacheAside`, `TestDelPrefix`,
`TestHandleGetEmployee` (fake gRPC client → 200 + JSON).

**Prove it**
```bash
curl -sD- -o/dev/null localhost:8080/api/v1/employees | grep -i x-cache   # MISS
curl -sD- -o/dev/null localhost:8080/api/v1/employees | grep -i x-cache   # HIT
```

---

## Phase 6 · Kafka events + transactional outbox relay

**Goal:** publish events reliably; consume with at-least-once semantics.

**Concepts:** the dual-write problem; outbox + relay; producer/consumer groups;
manual offset commit; propagating trace context through message headers.

**Create**
- `pkg/broker/broker.go` — producer, consumer, header carrier, trace inject/extract.
- `services/employee/internal/outbox/relay.go` — poll, publish, mark.

**Write these tests:** an integration test that inserts an outbox row and asserts
the relay publishes it (Redpanda container), and an idempotency test for the
consumer (redelivery does not double-apply).

**Prove it**
```bash
docker compose stop redpanda
curl -s -XPOST localhost:8080/api/v1/employees -H 'Content-Type: application/json' -d '{...}'
psql ... -c 'select count(*) from outbox where published_at is null;'   # grows
docker compose start redpanda                                            # relay drains it
```

---

## Phase 7 · Leave service: validation + approvals  ← the centerpiece

**Goal:** the domain logic — validate a leave request, persist it, and process
approvals; emit events.

**Concepts:** business-rule validation; working-day calculation; overlap checks;
balance accounting; calling another service over gRPC; optimistic concurrency on
approval; event emission in the same transaction.

**Create** `services/leave/...` implementing `LeaveService`:
- `workingDays(start, end, holidays)` — exclude weekends + public holidays.
- `ValidateLeaveRequest` — the 8 rules from the top of this file; return a
  structured error listing every violated rule (not just the first).
- `SubmitLeaveRequest` — validate → persist `requested` + outbox `leave.requested`.
- `DecideLeaveRequest` — approve/reject inside a tx: update status, adjust
  `leave_balances.used_days`, outbox `leave.decided`. Guard against double-decide.
- `GetLeaveBalance`, `ListLeaveRequests`.

**Write these tests (table-driven — this is the fun part)**
- `TestWorkingDays` (weekend span, holiday inside, single day)
- `TestValidateLeaveRequest` — one case per rule (overlap, insufficient balance,
  zero working days, past dates, too much notice, unknown type, inactive employee)
- `TestDecideLeaveRequest` adjusts balance and rejects a second decision.

**Prove it**
```bash
# submit a valid request
curl -s -XPOST localhost:8080/api/v1/leave-requests -H 'Content-Type: application/json' \
  -d '{"employee_id":"<uuid>","leave_type":"annual","start_date":"2026-07-01","end_date":"2026-07-10","reason":"holiday"}'
# submit an overlapping one -> expect 4xx with machine-readable reasons
curl -s -XPOST localhost:8080/api/v1/leave-requests -d '{...overlapping...}'
# approve, then try to approve again -> second call must fail
```

---

## Phase 8 · Reporting service (Redis counters + Kafka consumer)

**Goal:** fast read models from events — a CQRS-lite split.

**Concepts:** Redis counters + sorted sets; cache-aside; consumer groups;
at-least-once and idempotent projection updates.

**Create** `services/reporting/...` implementing `ReportingService`:
- a Kafka consumer handling `leave.requested`/`leave.decided`/`employee.created`
- Redis hashes/sorted sets: pending requests per department, leave days taken per
  month, top employees by leave used
- `GetDepartmentSummary`, `GetPendingApprovals`, `GetLeaveUsage`
- optional: persist a durable projection to the reporting DB.

**Write these tests:** a projection-handler unit test (same event applied twice is
idempotent) and a Redis-backed integration test.

**Prove it:** submit + approve requests and watch the department summary change in
Redis instantly, then replay the Kafka topic from the beginning and confirm the
projection does not double-count.

---

## Phase 9 · Kubernetes with kind + Kustomize

**Goal:** run the whole platform on a real cluster.

**Concepts:** Deployments/Services/StatefulSets; ConfigMap/Secret; probes; PVCs;
Ingress; HPA; Kustomize base + overlays.

**You build** `deploy/k8s/base/*` and `deploy/k8s/overlays/{dev,prod}/*`.
In-cluster: Postgres (init SQL for the 3 databases), Redis, Redpanda, otel-lgtm,
the four services, the web app, an Ingress and HPAs.

- `make kind-up` (or `kind create cluster --config deploy/kind/kind-config.yaml`)
- install ingress-nginx + metrics-server, `docker build` + `kind load`, `kubectl apply -k`.

**Guide:** [docs/03-kubernetes-kind.md](docs/03-kubernetes-kind.md).

**Know it works:** `http://hr.localhost:8088` serves the UI; Grafana shows
in-cluster traces.

---

## Phase 10 · Terraform → Floci

**Goal:** provision AWS-shaped resources locally and learn IaC.

**You build** `infra/terraform/floci/*` (S3 for archived leave documents, SQS +
DLQ for async decisions, SNS for notifications, DynamoDB for approval idempotency,
ECR for images) and optionally `eks-floci/*`.

- `docker compose --profile floci up -d floci`
- `terraform init && terraform apply`; verify with the AWS CLI at `:4566`.

**Guide:** [docs/05-terraform-floci.md](docs/05-terraform-floci.md).

---

## Phase 11 · Ansible

**Goal:** bootstrap the host and deploy, idempotently.

**You build** `infra/ansible/` (inventory, playbooks, roles) to install Docker,
Go, sqlc/buf, kubectl/kind/helm and to create the cluster + deploy.

- Re-running `bootstrap.yml` must report `changed=0`.

**Guide:** [docs/06-ansible.md](docs/06-ansible.md).

---

## Phase 12 · CI/CD

**Goal:** automate lint, test, codegen drift, image build/push and deploy.

**You build** `.github/workflows/ci.yml` + `cd.yml`, and `.golangci.yml`.

**Guide:** [docs/07-cicd.md](docs/07-cicd.md).

**Know it works:** a green pipeline that builds and pushes images to GHCR.

---

## Phase 13 · Testing & hardening

**Goal:** prove the system under failure.

- Integration tests: employee + leave against real Postgres; leave → employee over
  gRPC with `bufconn`; the reporting consumer against real Postgres + Redpanda.
- Concurrency: two approvals racing for the same request (only one wins).
- Break each dependency and observe the pattern (see
  [docs/08-distributed-patterns.md](docs/08-distributed-patterns.md) "Prove it"
  steps): breaker opens, Kafka offsets stall, outbox backlog drains.
- Add retries/backoff and the DynamoDB idempotency table for approvals.

---

## Cheat sheet

```bash
make test              # unit tests (your progress bar)
make test-integration  # Testcontainers
make generate          # buf + sqlc
make dev-up dev-down   # local infra
make run-employee run-leave run-reporting run-gateway
make kind-up kind-load deploy-dev
```

Full reading is in `docs/` (architecture, observability, k8s, terraform, ansible,
ci/cd, patterns) and the 2-day schedule in `docs/01-two-day-plan.md`.
