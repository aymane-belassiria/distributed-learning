# v2 · Deep track + final project

**Prerequisite:** a working v1 slice ([../v1/LEARNING_PATH.md](../v1/LEARNING_PATH.md)).
v2 takes the same system and adds the operational depth and distributed-systems
depth — then assembles the polished final project.

Each phase links to a reading doc in this folder. You still write the code and
config yourself; the docs explain and give exercises.

---

## Checklist

- [ ] v2.0 — Full observability (OpenTelemetry → Tempo/Prometheus/Loki/Grafana)
- [ ] v2.1 — Resilience: circuit breaker + retries/backoff
- [ ] v2.2 — Distributed databases (indexes, plans, isolation, replication, sharding)
- [ ] v2.3 — Harden eventing: idempotency, at-least-once, durable CQRS
- [ ] v2.4 — Kubernetes with kind + Kustomize
- [ ] v2.5 — Terraform → Floci (local AWS + EKS)
- [ ] v2.6 — Ansible (idempotent host bootstrap + deploy)
- [ ] v2.7 — CI/CD (GitHub Actions)
- [ ] v2.8 — React UI
- [ ] v2.9 — Final project: assemble, harden, document
- [ ] v2.10 — Extensions (optional)

---

## v2.0 · Full observability

**Goal:** traces, metrics and correlated logs across HTTP → gRPC → Kafka.

**Do**
- Build `pkg/otelx` (TracerProvider + MeterProvider + OTLP exporters + W3C
  propagation) and a `pkg/logger` handler that injects `trace_id`/`span_id`.
- Instrument HTTP (`otelhttp`), gRPC (`otelgrpc`), Postgres (pgx), Redis, and
  inject `traceparent` into Kafka headers.
- Add the LGTM stack to your compose **only now** (Grafana 3300, Prometheus 9099,
  Loki 3101, Tempo 3201, OTLP 4317/4318).

**Prove it:** submit a leave request and find one Tempo trace containing
`SubmitLeaveRequest` → `employee.GetEmployee` → the DB query → the Kafka publish.

**Guide:** [04-observability.md](04-observability.md)

---

## v2.1 · Resilience: circuit breaker + retries

**Goal:** stop calling a failing dependency; recover automatically.

**Do**
- Build `pkg/breaker` (closed → open → half-open) over a small state machine.
- Wrap `leave → employee` and `gateway → services` calls.
- Add retries with backoff + jitter for transient errors (and *no* retries for
  validation errors).

**Prove it:** stop `employee`, fire requests, watch the breaker open and calls
fail fast (503); restart `employee`, watch the half-open probe recover it.

**Guide:** [08-distributed-patterns.md](08-distributed-patterns.md) (patterns 6)

---

## v2.2 · Distributed databases

**Goal:** make queries fast and correct, and understand multi-node reality.

**Do**
- `EXPLAIN (ANALYZE, BUFFERS)`, `pg_stat_statements`, `auto_explain`.
- Index the overlap query (`idx_leave_overlap`); force a bad plan; fix it.
- Add the `btree_gist` **exclusion constraint**; map `23P01` → 409.
- The approval race: optimistic status guard vs `SELECT … FOR UPDATE` vs
  `SERIALIZABLE` + retry. Reproduce a deadlock and fix lock ordering.
- Partition `leave_requests` by year; stand up a read replica and measure lag.

**Prove it:** two concurrent approvals → exactly one wins, balance moves once.

**Guide:** [09-databases.md](09-databases.md) (10 graded exercises)

---

## v2.3 · Harden eventing

**Goal:** correctness under redelivery and failure.

**Do**
- Consumer commits offsets only after a successful projection update; handle
  poison messages.
- Make projection updates idempotent (event id / status guard); add the
  DynamoDB-backed idempotency table for approvals.
- Make the reporting read model durable (CQRS), not just Redis counters.

**Prove it:** replay a topic from the beginning → no double counting.

**Guide:** [08-distributed-patterns.md](08-distributed-patterns.md) (patterns 4, 7)

---

## v2.4 · Kubernetes with kind + Kustomize

**Goal:** run the whole platform on a real cluster.

**Do**
- `kind` single-node cluster; install ingress-nginx + metrics-server.
- Kustomize `base/` + `overlays/{dev,prod}`: Deployments/Services, Postgres
  StatefulSet (PVC + 3-database init), Redis, Redpanda, LGTM, Ingress, HPAs.

**Prove it:** `http://hr.localhost:8088` serves the API; Grafana shows in-cluster
traces.

**Guide:** [03-kubernetes-kind.md](03-kubernetes-kind.md)

---

## v2.5 · Terraform → Floci

**Goal:** provision AWS-shaped resources locally and learn IaC.

**Do**
- `docker compose --profile floci up -d floci` (local AWS on :4566).
- S3 (leave attachments), SQS + DLQ (async decisions), SNS (notifications),
  DynamoDB (approval idempotency), ECR (images).
- Then the EKS module: a real k3s control plane via Floci.

**Prove it:** `terraform output` lists resources; AWS CLI against :4566 sees them;
`aws eks update-kubeconfig` works.

**Guide:** [05-terraform-floci.md](05-terraform-floci.md)

---

## v2.6 · Ansible

**Goal:** idempotent host bootstrap + deploy.

**Do**
- Roles: `docker`, `golang`, `devtools` (sqlc/buf/kubectl/kind/helm), `kind`.
- `bootstrap.yml` and `deploy.yml`.

**Prove it:** run `bootstrap.yml` twice; the second run reports `changed=0`.

**Guide:** [06-ansible.md](06-ansible.md)

---

## v2.7 · CI/CD

**Goal:** automate lint, test, codegen drift, image build/push, deploy.

**Do**
- `ci.yml`: `go build/vet/test`, `golangci-lint`, codegen-drift (`buf`+`sqlc`),
  integration tests (Testcontainers), Docker build matrix, `terraform validate`,
  `ansible --syntax-check`, `kubectl kustomize` + `kubeconform`, web build.
- `cd.yml`: push images to GHCR, deploy on `main` (self-hosted runner or SSH).

**Guide:** [07-cicd.md](07-cicd.md)

---

## v2.8 · React UI

**Goal:** the only client.

**Do**
- Vite + TypeScript app talking only to the gateway: employee directory, submit
  a leave request (with validation errors surfaced), approve/reject, department
  reports.
- Containerize it; serve it via the k8s Ingress.

---

## v2.9 · Final project: assemble, harden, document

**Goal:** the polished whole system.

- Every service deployed on kind; observability live; CI/CD green.
- Integration + concurrency tests; break-each-dependency drills
  ([08-distributed-patterns.md](08-distributed-patterns.md) "Prove it" sections).
- A `README` with the architecture diagram, a one-command run, and a short "what
  I'd change for production".
- **Reference:** [01-two-day-plan.md](01-two-day-plan.md) for the condensed
  schedule, [00-architecture.md](00-architecture.md) for the full design.

---

## v2.10 · Extensions (optional)

- Holiday-calendar API feeding `workingDays`.
- Monthly leave accrual as a Kubernetes **CronJob**.
- Multi-level approvals (manager → HR).
- Helm chart instead of Kustomize.
- Grafana alerts (breaker open, approvals backlog, p99 latency).
- Load test with `k6`/`hey` and watch the HPA scale.
