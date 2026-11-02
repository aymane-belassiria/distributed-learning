# v2 · Deep track + final project

**Prerequisite:** a working v1 slice ([../v1/LEARNING_PATH.md](../v1/LEARNING_PATH.md)).
v2 takes the same system and adds the operational depth and distributed-systems
depth — then assembles the polished final project.

**Session size:** like v1, every ticket is **one sitting (~1 hour)** and ends on a
checkpoint you can verify. Each phase links to a reading doc in this folder; you
still write the code and config yourself.

---

## Checklist

- [ ] **v2.0** — Full observability
  - [ ] v2.0a — `pkg/otelx` providers + propagation
  - [ ] v2.0b — instrument HTTP + gRPC
  - [ ] v2.0c — instrument DB + Redis + Kafka headers
  - [ ] v2.0d — LGTM compose + find one trace
- [ ] **v2.1** — Resilience
  - [ ] v2.1a — `pkg/breaker` + tests
  - [ ] v2.1b — wrap the gRPC/HTTP calls
  - [ ] v2.1c — retries with backoff + jitter
  - [ ] v2.1d — outage drill (open → half-open)
- [ ] **v2.2** — Distributed databases
  - [ ] v2.2a — `EXPLAIN`, `pg_stat_statements`, `auto_explain`
  - [ ] v2.2b — index the overlap query
  - [ ] v2.2c — `btree_gist` exclusion constraint
  - [ ] v2.2d — approval race (optimistic vs `FOR UPDATE` vs `SERIALIZABLE`)
  - [ ] v2.2e — deadlock + lock ordering
  - [ ] v2.2f — partition `leave_requests` by year
  - [ ] v2.2g — read replica + lag
- [ ] **v2.3** — Harden eventing
  - [ ] v2.3a — commit offsets after projection; poison messages
  - [ ] v2.3b — idempotent projections
  - [ ] v2.3c — DynamoDB idempotency table
  - [ ] v2.3d — durable CQRS read model
  - [ ] v2.3e — replay drill
- [ ] **v2.4** — Kubernetes with kind + Kustomize
  - [ ] v2.4a — cluster + ingress-nginx + metrics-server
  - [ ] v2.4b — Kustomize base (the four services)
  - [ ] v2.4c — stateful deps (Postgres/Redis/Redpanda)
  - [ ] v2.4d — LGTM + Ingress + HPA
  - [ ] v2.4e — prove it via `hr.localhost:8088`
- [ ] **v2.5** — Terraform → Floci
  - [ ] v2.5a — Floci up + provider
  - [ ] v2.5b — S3 + SQS/DLQ + SNS
  - [ ] v2.5c — DynamoDB + ECR
  - [ ] v2.5d — EKS module + kubeconfig
- [ ] **v2.6** — Ansible
  - [ ] v2.6a — inventory + docker role
  - [ ] v2.6b — golang + devtools role
  - [ ] v2.6c — kind role + idempotent `bootstrap.yml`
  - [ ] v2.6d — `deploy.yml`
- [ ] **v2.7** — CI/CD
  - [ ] v2.7a — `ci.yml` build/vet/test/lint
  - [ ] v2.7b — codegen-drift job
  - [ ] v2.7c — integration tests (Testcontainers)
  - [ ] v2.7d — build matrix + terraform/ansible/kustomize checks + web build
  - [ ] v2.7e — `cd.yml` (GHCR push + deploy)
- [ ] **v2.8** — React UI
  - [ ] v2.8a — Vite + TS client
  - [ ] v2.8b — employee directory + create
  - [ ] v2.8c — leave request form
  - [ ] v2.8d — approve/reject + reports
  - [ ] v2.8e — containerize + serve via Ingress
- [ ] **v2.9** — Final project
  - [ ] v2.9a — everything on kind
  - [ ] v2.9b — break-each-dependency drills
  - [ ] v2.9c — concurrency tests
  - [ ] v2.9d — README + "what I'd change for production"
- [ ] **v2.10** — Extensions (optional)

---

## v2.0 · Full observability

**Goal:** traces, metrics and correlated logs across HTTP → gRPC → Kafka.

### v2.0a · `pkg/otelx` · ⏱ ~1h
**Build:** `TracerProvider` + `MeterProvider` + OTLP exporters + W3C
propagation; a `pkg/logger` handler that injects `trace_id`/`span_id`.
**Done when:** a tiny main emits a span and it reaches the collector.

### v2.0b · instrument HTTP + gRPC · ⏱ ~1h
**Build:** `otelhttp` on the gateway, `otelgrpc` on the services.
**Done when:** one gateway call produces a parent span with a gRPC child span.

### v2.0c · instrument DB + Redis + Kafka · ⏱ ~1h
**Build:** pgx + Redis instrumentation; inject `traceparent` into Kafka headers.
**Done when:** a DB query and a produce appear as children of the same trace.

### v2.0d · LGTM compose + find one trace · ⏱ ~1h
**Build:** add the LGTM stack to compose **only now** (Grafana 3300, Prometheus
9099, Loki 3101, Tempo 3201, OTLP 4317/4318).
**Done when:** one leave request yields a single Tempo trace containing
`SubmitLeaveRequest` → `employee.GetEmployee` → the DB query → the Kafka publish.
**Guide:** [04-observability.md](04-observability.md)

---

## v2.1 · Resilience: circuit breaker + retries

### v2.1a · `pkg/breaker` + tests · ⏱ ~1h
**Build:** a closed → open → half-open state machine.
**Done when:** unit tests drive it through all three states.

### v2.1b · wrap the calls · ⏱ ~1h
**Build:** wrap `leave → employee` and `gateway → services`.
**Done when:** a forced failure trips the breaker and stops the calls.

### v2.1c · retries + backoff · ⏱ ~1h
**Build:** backoff + jitter for transient errors, and **no** retries for
validation errors.
**Done when:** a transient failure is retried; an `InvalidArgument` is not.

### v2.1d · outage drill · ⏱ ~1h
**Build:** no code — run the drill.
**Done when:** stop `employee` → calls fail fast (503) → restart → the half-open
probe recovers it.
**Guide:** [08-distributed-patterns.md](08-distributed-patterns.md) (pattern 6)

---

## v2.2 · Distributed databases

**Goal:** make queries fast and correct, and understand multi-node reality.

### v2.2a · plans + stats · ⏱ ~1h
**Build:** `EXPLAIN (ANALYZE, BUFFERS)`, turn on `pg_stat_statements` and
`auto_explain`.
**Done when:** you can read a plan and name the slow node.

### v2.2b · index the overlap query · ⏱ ~1h
**Build:** add `idx_leave_overlap`; force a bad plan, then fix it.
**Done when:** the plan changes from seq scan to index scan and the time drops.

### v2.2c · exclusion constraint · ⏱ ~1h
**Build:** `btree_gist` exclusion constraint; map SQLSTATE `23P01` → 409.
**Done when:** an overlapping insert is rejected at the DB and surfaces as 409.

### v2.2d · the approval race · ⏱ ~1h
**Build:** compare optimistic status guard vs `SELECT … FOR UPDATE` vs
`SERIALIZABLE` + retry.
**Done when:** two concurrent approvals → exactly one wins, balance moves once.

### v2.2e · deadlock + lock ordering · ⏱ ~1h
**Build:** reproduce a deadlock, then fix it with consistent lock ordering.
**Done when:** the reproduction stops deadlocking under the fix.

### v2.2f · partition by year · ⏱ ~1h
**Build:** partition `leave_requests` by year.
**Done when:** a year's partition prunes in the plan.

### v2.2g · read replica + lag · ⏱ ~1h
**Build:** stand up a replica; measure replication lag.
**Done when:** you can show reads landing on the replica with measured lag.
**Guide:** [09-databases.md](09-databases.md) (10 graded exercises)

---

## v2.3 · Harden eventing

**Goal:** correctness under redelivery and failure.

### v2.3a · offsets + poison messages · ⏱ ~1h
**Build:** commit offsets only after a successful projection update; handle
poison messages.
**Done when:** a failing message doesn't lose the offsets behind it.

### v2.3b · idempotent projections · ⏱ ~1h
**Build:** event id / status guard so replays don't double-count.
**Done when:** replaying the same batch changes nothing.

### v2.3c · DynamoDB idempotency table · ⏱ ~1h
**Build:** an idempotency table (Floci/local DynamoDB) for approvals.
**Done when:** a duplicate approval attempt is rejected via the table.

### v2.3d · durable CQRS read model · ⏱ ~1h
**Build:** make the reporting read model durable (a real table), not just Redis
counters.
**Done when:** the model survives a reporting restart.

### v2.3e · replay drill · ⏱ ~1h
**Build:** no code — replay a topic from the beginning.
**Done when:** no double counting.
**Guide:** [08-distributed-patterns.md](08-distributed-patterns.md) (patterns 4, 7)

---

## v2.4 · Kubernetes with kind + Kustomize

**Goal:** run the whole platform on a real cluster.

### v2.4a · cluster + ingress + metrics · ⏱ ~1h
**Build:** a `kind` single-node cluster; install ingress-nginx + metrics-server.
**Done when:** `kubectl get nodes` is Ready and ingress has an address.

### v2.4b · Kustomize base (services) · ⏱ ~1h
**Build:** `base/` Deployments + Services for the four apps.
**Done when:** `kubectl kustomize base` renders valid manifests.

### v2.4c · stateful deps · ⏱ ~1h
**Build:** Postgres StatefulSet (PVC + 3-database init), Redis, Redpanda.
**Done when:** the apps can reach the deps in-cluster.

### v2.4d · LGTM + Ingress + HPA · ⏱ ~1h
**Build:** LGTM stack, Ingress, HPAs, plus `overlays/{dev,prod}`.
**Done when:** the Ingress route serves the API and an HPA exists.

### v2.4e · prove it · ⏱ ~1h
**Done when:** `http://hr.localhost:8088` serves the API and Grafana shows
in-cluster traces.
**Guide:** [03-kubernetes-kind.md](03-kubernetes-kind.md)

---

## v2.5 · Terraform → Floci

**Goal:** provision AWS-shaped resources locally and learn IaC.

### v2.5a · Floci up + provider · ⏱ ~1h
**Build:** `docker compose --profile floci up -d floci` (local AWS on :4566);
the AWS provider pointed at it.
**Done when:** `terraform apply` of a trivial resource succeeds.

### v2.5b · S3 + SQS/DLQ + SNS · ⏱ ~1h
**Build:** S3 (leave attachments), SQS + DLQ (async decisions), SNS
(notifications).
**Done when:** `terraform output` lists them and the AWS CLI sees them.

### v2.5c · DynamoDB + ECR · ⏱ ~1h
**Build:** the DynamoDB idempotency table and an ECR repo.
**Done when:** both exist via `aws --endpoint-url :4566`.

### v2.5d · EKS module · ⏱ ~1h
**Build:** the EKS module (a real k3s control plane via Floci).
**Done when:** `aws eks update-kubeconfig` works and `kubectl get nodes` responds.
**Guide:** [05-terraform-floci.md](05-terraform-floci.md)

---

## v2.6 · Ansible

**Goal:** idempotent host bootstrap + deploy.

### v2.6a · inventory + docker role · ⏱ ~1h
**Build:** inventory + a `docker` role.
**Done when:** running the role installs/verifies Docker.

### v2.6b · golang + devtools role · ⏱ ~1h
**Build:** `golang`, `devtools` (sqlc/buf/kubectl/kind/helm).
**Done when:** all tools are present after a run.

### v2.6c · kind role + bootstrap · ⏱ ~1h
**Build:** the `kind` role + `bootstrap.yml`.
**Done when:** running `bootstrap.yml` **twice** reports `changed=0` the second
time.

### v2.6d · deploy.yml · ⏱ ~1h
**Build:** `deploy.yml`.
**Done when:** a deploy run rolls out the app.
**Guide:** [06-ansible.md](06-ansible.md)

---

## v2.7 · CI/CD

**Goal:** automate lint, test, codegen drift, image build/push, deploy.

### v2.7a · `ci.yml` basics · ⏱ ~1h
**Build:** `go build/vet/test` + `golangci-lint`.
**Done when:** the workflow is green on `main`.

### v2.7b · codegen-drift job · ⏱ ~1h
**Build:** run `buf` + `sqlc` and fail if the generated code differs.
**Done when:** touching a `.proto` without regenerating fails CI.

### v2.7c · integration tests · ⏱ ~1h
**Build:** Testcontainers-backed tests in CI.
**Done when:** the integration job passes.

### v2.7d · the rest of the matrix · ⏱ ~1h
**Build:** Docker build matrix, `terraform validate`, `ansible --syntax-check`,
`kubectl kustomize` + `kubeconform`, web build.
**Done when:** every check runs in one workflow.

### v2.7e · `cd.yml` · ⏱ ~1h
**Build:** push images to GHCR, deploy on `main` (self-hosted runner or SSH).
**Done when:** a merge to `main` deploys.
**Guide:** [07-cicd.md](07-cicd.md)

---

## v2.8 · React UI (the only client)

Each ticket builds only on the gateway REST API.

### v2.8a · Vite + TS client · ⏱ ~1h
**Build:** scaffold the app + a typed gateway client.
**Done when:** it lists employees from the live gateway.

### v2.8b · employee directory + create · ⏱ ~1h
**Done when:** you can browse and create an employee from the UI.

### v2.8c · leave request form · ⏱ ~1h
**Done when:** the form surfaces validation reasons from the API.

### v2.8d · approve/reject + reports · ⏱ ~1h
**Done when:** you can approve/reject and see department reports.

### v2.8e · containerize + serve · ⏱ ~1h
**Done when:** the UI is served through the k8s Ingress.

---

## v2.9 · Final project: assemble, harden, document

### v2.9a · everything on kind · ⏱ ~1h
**Done when:** every service is deployed and observability is live.

### v2.9b · break-each-dependency drills · ⏱ ~1h
**Build:** run the "Prove it" drills from
[08-distributed-patterns.md](08-distributed-patterns.md).
**Done when:** each drill behaves as predicted.

### v2.9c · concurrency tests · ⏱ ~1h
**Done when:** the approval race + replay tests are in CI.

### v2.9d · README + retro · ⏱ ~1h
**Build:** a `README` with the architecture diagram, a one-command run, and a
short "what I'd change for production".
**Reference:** [01-two-day-plan.md](01-two-day-plan.md),
[00-architecture.md](00-architecture.md)

---

## v2.10 · Extensions (optional)

- Holiday-calendar API feeding `workingDays`.
- Monthly leave accrual as a Kubernetes **CronJob**.
- Multi-level approvals (manager → HR).
- Helm chart instead of Kustomize.
- Grafana alerts (breaker open, approvals backlog, p99 latency).
- Load test with `k6`/`hey` and watch the HPA scale.
