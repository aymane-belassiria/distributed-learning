# 00 · Architecture

## The idea in one paragraph

An HR platform where staff manage employees and employees request **leave** that
gets validated against real business rules (no overlaps, working-days only,
enough balance) and approved by a manager. Four Go services behind one REST
gateway, talking over gRPC and through Kafka, sharing Postgres (one database per
service) and Redis. A React app is the only client. Everything is instrumented
with OpenTelemetry. The point is not the HR features — it is that **every box is
a technology you can point at and explain**, and the leave rules are real logic
worth testing.

## Services and what they own

| Service     | Owns (writes)                                  | Reads                      | Exposes |
| ----------- | ---------------------------------------------- | -------------------------- | ------- |
| `employee`  | `employee` DB (employees, departments)         | —                          | gRPC    |
| `leave`     | `leave` DB (requests, balances, holidays)      | `employee` over gRPC       | gRPC    |
| `reporting` | `reporting` DB (projections) + Redis           | Kafka, Redis               | gRPC    |
| `gateway`   | nothing (stateless BFF)                        | all three over gRPC, Redis | HTTP    |

**Database-per-service:** one Postgres server, three logical databases
(`employee`, `leave`, `reporting`), each migrated by its owning service from an
embedded migration directory. No service reads another's tables.

## Data model

```
departments   (id, code, name, created_at)
employees     (id, employee_code, first_name, last_name, email, department_id,
               position, manager_id, hire_date, employment_type, status, created_at)
leave_types   (code, name, default_days_per_year, requires_balance)
holidays      (id, country, day, name)
leave_requests(id, employee_id, leave_type, start_date, end_date, working_days,
               status, reason, requested_at, decided_at, decided_by, decision_note)
leave_balances(employee_id, leave_type, year, entitled_days, used_days)
outbox        (id, topic, key, payload, created_at, published_at)
```

`status` for a request moves `requested → approved | rejected | cancelled`.
`working_days` is stored (not recomputed) so a report never disagrees with a
decision.

## Request flows

### 1. Browse the directory (read path, cache-aside)

```
browser → gateway GET /api/v1/employees?department=eng
        → Redis GET hr:employees:...
          hit  → return (X-Cache: HIT)
          miss → breaker → employee.ListEmployees (gRPC) → Postgres
               → Redis SET (TTL 30s) → return (X-Cache: MISS)
```

### 2. Submit a leave request (validation + event)

```
browser → gateway POST /api/v1/leave-requests
        → breaker → leave.SubmitLeaveRequest (gRPC)
             1. employee.GetEmployee(id)            (gRPC, must be active)
             2. workingDays(start,end,holidays)     (exclude weekends/holidays)
             3. check overlap, balance, notice, max-consecutive…
             4. ONE tx: INSERT leave_request(status=requested) + INSERT outbox
                   topic leave.requested
outbox relay → Kafka produce leave.requested
```

### 3. Decide a request (approval, with balance accounting)

```
browser → gateway POST /api/v1/leave-requests/{id}/decide {approve:true}
        → breaker → leave.DecideLeaveRequest (gRPC)
             tx: UPDATE status (guard: only if currently `requested`)
                 UPDATE leave_balances.used_days += working_days
                 INSERT outbox topic leave.decided
             commit → relay → Kafka
```

### 4. Reporting (read model built from events)

```
Kafka leave.requested / leave.decided / employee.created
   → reporting consumer (consumer group)
        ├─ Redis HINCRBY hr:reporting:dept:{id} pending/approved
        └─ (optional) upsert a durable projection row
gateway GET /api/v1/reports/departments → reporting.GetDepartmentSummary
```

## Why these choices

- **sqlc, not an ORM.** You write SQL, sqlc generates typed Go. Map
  `timestamptz`→`time.Time`, `uuid`→`uuid.UUID`, `jsonb`→`json.RawMessage`.
- **gRPC between services, REST at the edge.** Contracts in `proto/hr/v1`,
  codegen via `buf`.
- **Kafka for events.** Decouples "record a decision" from "update reports".
  `reporting` is a consumer group — scale it and partitions rebalance.
- **The gateway holds the resilience.** Circuit breakers, cache, CORS and HTTP in
  one place; domain services stay small and typed.
- **Validation lives in `leave`, not the gateway.** Domain rules belong with the
  data they protect, and they are unit-testable without any network.
- **OTLP everywhere.** Services push to a Collector that fans out to
  Tempo/Prometheus/Loki; Grafana is the single pane.

## The leave validation rules

`SubmitLeaveRequest` rejects (with **all** reasons, not just the first) when:

1. the employee is missing or not `active`
2. `start_date > end_date`, or the whole range is in the past
3. the leave type is unknown
4. the range contains zero working days (weekends/holidays excluded)
5. working days exceed the max consecutive allowance
6. it overlaps an existing non-cancelled request
7. balance is insufficient for a balance-bearing leave type
8. the notice period is too short (annual leave)

Because these are pure functions over inputs (dates + existing rows + a holiday
list), they make excellent table-driven tests — see Phase 7.

## Failure modes to explore (Phase 13 exercises)

| Break this                       | Observe                                             |
| -------------------------------- | --------------------------------------------------- |
| Stop `employee`; submit leave    | `leave` breaker opens → request fails fast (503)    |
| Stop Redis in the gateway        | `/readyz` fails, reads fall back to Postgres        |
| Stop Redpanda                    | outbox rows accumulate; decisions still committed   |
| Kill `reporting` mid-consume     | offsets not committed → events redelivered          |
| Approve the same request twice   | second decision rejected (status guard)             |
| Two approvals race               | only one wins (transaction + status guard)          |

## Layout you will build

| Concept | Package |
| ------- | ------- |
| OTel SDK setup | `pkg/otelx` |
| Trace-correlated JSON logs | `pkg/logger` |
| Cache-aside + prefix invalidation | `pkg/cache` |
| Circuit breaker | `pkg/breaker` |
| Kafka produce/consume with trace propagation | `pkg/broker` |
| gRPC server/client + health | `pkg/grpcx` |
| Employee CRUD + outbox | `services/employee` |
| Leave validation + approvals + outbox | `services/leave` |
| Read model from events | `services/reporting` |
| REST BFF, breakers, cache | `services/gateway` |
