# 01 · The two-day plan

Two days is enough for the whole vertical slice if you follow the order below.
Each block ends with a **checkpoint** you can actually run. Don't move on until
the checkpoint passes.

> Keep `make dev-up` running and four `make run-*` terminals (or `tmux`).
> Re-run `make generate` whenever you touch `proto/` or a `.sql` query.

---

## Day 1 — the distributed system, locally

### Block 1 · Toolchain & infra (30–45 min)
- Install Go 1.24+, make, Docker; then sqlc, buf, and the protoc plugins.
- `make dev-up` (Postgres, Redis, Redpanda, LGTM).
- Skim `00-architecture.md` and the data model.

**Checkpoint:** `docker compose ps` shows four healthy containers; Grafana on
http://localhost:3300 loads.

### Block 2 · Contracts & codegen (45 min)
- Write `proto/hr/v1/{employee,leave,reporting}.proto`. Run `buf lint`.
- Write `sqlc.yaml` and the employee migrations/queries.
- Change a query, run `make generate`, see the generated Go change.

**Checkpoint:** `make generate` is clean; `go build ./...` succeeds.

### Block 3 · Employee service, then run it (75 min)
- Implement `employee` (sqlc + outbox), wire migrations + `main.go`.
- `make run-employee`; `grpcurl` a `CreateEmployee`.
- Write a Testcontainers integration test.

**Checkpoint:** `CREATE` returns an employee; the `outbox` table has a row;
`make test-integration` passes.

### Block 4 · Observability (60 min)
- Implement `pkg/logger` (trace correlation) and `pkg/otelx` (providers).
- Follow one request across services in the JSON logs.

**Checkpoint:** Grafana → Tempo shows a trace for your `CreateEmployee` call; a
log line's `trace_id` matches it.

### Block 5 · gRPC + circuit breaker (60 min)
- Implement `pkg/grpcx` (interceptors, health, reflection) and `pkg/breaker`.
- Install `grpcurl` and call the employee service through the gateway.

**Checkpoint:** breaker tests pass; stopping `employee` makes calls fail fast.

### Block 6 · Gateway + cache, then patterns (90 min)
- Implement the gateway routes + Redis cache-aside; watch `X-Cache` flip.
- Implement `pkg/broker` + the employee outbox relay.
- Stop Redpanda, onboard an employee, restart Redpanda, watch the relay drain.

**Checkpoint:** you can explain outbox vs. direct publish, and cache-aside
invalidation.

### Block 7 · Leave validation (90 min) ← the centerpiece
- Implement `workingDays`, `ValidateLeaveRequest`, `SubmitLeaveRequest`,
  `DecideLeaveRequest`.
- Write the table-driven tests — one case per rule, plus the double-decide guard.

**Checkpoint:** all validation tests green; an overlapping request is rejected
with a machine-readable list of reasons; approving adjusts the balance.

### Block 7b · Databases: plans, indexes & constraints (45–60 min, optional)
- Follow [09-databases.md](09-databases.md) Parts A–C.
- Turn on `pg_stat_statements` + `auto_explain`; `EXPLAIN (ANALYZE, BUFFERS)` the
  overlap query; add `idx_leave_overlap` and confirm the node changes.
- Add the `btree_gist` **exclusion constraint** and map `23P01` → 409.
- **Checkpoint:** the overlap query uses an index; two concurrent overlapping
  inserts leave exactly one winner.

> The distributed half of [09-databases.md](09-databases.md) (replication,
> sharding, CAP/PACELC, 2PC vs Sagas) is stretch reading — thread it in as you
> like, but don't put it on the two-day deadline.

---

## Day 2 — ops: Kubernetes, Terraform, Ansible, CI/CD

### Block 8 · Reporting service (60 min)
- Implement the Kafka consumer + Redis counters + `GetDepartmentSummary`.
- Submit + approve requests and watch the summary change; replay the topic and
  confirm idempotency.

**Checkpoint:** department reports update from events without double-counting.

### Block 9 · Kubernetes on kind (120 min)
- `kind create cluster --config deploy/kind/kind-config.yaml`
- install ingress-nginx + metrics-server; build + load images; `kubectl apply -k`.
- Explore rollouts, `rollout undo`, and the HPA.

**Checkpoint:** http://hr.localhost:8088 serves the UI; Grafana shows in-cluster
traces.

### Block 10 · Terraform → Floci (75 min)
- `docker compose --profile floci up -d floci`
- Provision S3 (archived leave docs), SQS(+DLQ) for async decisions, SNS
  (notifications), DynamoDB (approval idempotency), ECR.
- Optionally stand up EKS in Floci.

**Checkpoint:** `terraform output` lists your resources; `aws s3 ls
--endpoint-url http://localhost:4566` shows the bucket.

### Block 11 · Ansible (45 min)
- Write the inventory, roles and playbooks; run bootstrap twice.

**Checkpoint:** the second run reports `changed=0`.

### Block 12 · CI/CD (60 min)
- Write `ci.yml` (lint, test, codegen drift, build, terraform, ansible, k8s, web).
- Write `cd.yml` (push images to GHCR, deploy to a self-hosted runner).

**Checkpoint:** a green pipeline that builds and pushes images.

### Block 13 · Tests & polish (rest)
- Integration + concurrency tests (racing approvals).
- Break each dependency and watch the pattern recover.
- Write a short "what I'd change for production".

---

## Stretch goals

- **Holiday calendar API**: load public holidays and use them in `workingDays`.
- **Accrual**: monthly leave accrual as a scheduled job (Kubernetes CronJob).
- **Approval workflow**: multi-level approvals (manager → HR).
- **Helm chart** for the k8s base.
- **Grafana alerts**: breaker open, pending approvals backlog, p99 latency.
- **Load test** with `k6`/`hey` and watch the HPA.
