# v2 · Deep track + final project

The second half. Once the [v1 local slice](../v1/LEARNING_PATH.md) runs, this
track adds the operational and distributed-systems depth and ends with the
**final assembled project**.

Start at **[LEARNING_PATH.md](LEARNING_PATH.md)** (v2.0 → v2.10).

## Reading in this folder

| Doc | Topic |
| --- | ----- |
| [00-architecture.md](00-architecture.md) | the full system design, flows, failure modes |
| [01-two-day-plan.md](01-two-day-plan.md) | the condensed hour-by-hour schedule |
| [02-local-dev.md](02-local-dev.md) | running and debugging everything locally |
| [03-kubernetes-kind.md](03-kubernetes-kind.md) | kind cluster, Kustomize, operate it |
| [04-observability.md](04-observability.md) | OpenTelemetry → Tempo/Prometheus/Loki/Grafana |
| [05-terraform-floci.md](05-terraform-floci.md) | Terraform against the Floci AWS emulator |
| [06-ansible.md](06-ansible.md) | idempotent host bootstrap + deploy |
| [07-cicd.md](07-cicd.md) | GitHub Actions CI/CD |
| [08-distributed-patterns.md](08-distributed-patterns.md) | outbox, at-least-once, cache-aside, breaker, CQRS, tracing |
| [09-databases.md](09-databases.md) | indexes, query plans, isolation, replication, sharding |

## The final project (target)

A running HR platform: four Go services + React UI, deployed on Kubernetes
(kind, with a Terraform→Floci EKS path), fully observable, resilient
(circuit breaker, retries, idempotency), backed by correctly indexed Postgres,
with CI/CD and Ansible. See **v2.9** in [LEARNING_PATH.md](LEARNING_PATH.md).

> Reminder: these docs explain and set tasks. You write the code, config and
> tests. Build in small steps — minimum config first, then grow.
