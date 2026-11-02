# LEARNING_PATH — build the HR platform yourself

A two-track, build-it-yourself workflow for a **distributed HR / leave-management
platform** (sqlc, gRPC, Kafka, Redis, Kubernetes, Terraform, Ansible, CI/CD,
React, and the distributed patterns around them).

You write every file. This repo contains **only** the two tracks and their
reading docs — no code, no config, no answer key.

## The rule

**Start small, grow, minimum config first.** Add a container, a dependency, or a
layer only when the phase you are on needs it — then move to the deeper version.
Never copy the full stack up front.

## Session size — ~1 hour per ticket

Every ticket in both tracks is scoped to **one sitting (~1 hour), because this
gets done after work.** Each ticket ends on a green checkpoint you can see
working, so you can stop at any sitting without leaving a half-finished mess.
Phases keep their IDs (`v1.2`, `v2.4`) and are split into lettered sittings
(`v1.2a`, `v1.2b`, …). If a sitting starts to sprawl, stop at the last checkpoint
and pick up the next letter.

## Two tracks

### [v1 · The local slice → docs/v1/](docs/v1/README.md) — start here

Get the four HR services running **end to end on your machine**, smallest config
at a time: employee, leave (the validation logic), reporting, gateway; Postgres +
Redis + Redpanda.

👉 **[v1 task list](docs/v1/LEARNING_PATH.md)** (v1.0 → v1.8)

### [v2 · Deep track + final project → docs/v2/](docs/v2/README.md)

The same system, taken deep and shipped: OpenTelemetry observability, circuit
breaker + retries, distributed databases, hardened eventing, Kubernetes (kind →
Floci EKS), Terraform, Ansible, CI/CD, React UI, and the final assembled project.

👉 **[v2 task list](docs/v2/LEARNING_PATH.md)** (v2.0 → v2.10)

```
v1:  local infra → sqlc → gRPC services → gateway + cache → outbox/Kafka
     → leave validation → logging → reporting → runs end to end

v2:  observability → resilience → databases → eventing → Kubernetes
     → Terraform → Ansible → CI/CD → React UI → FINAL PROJECT
```

## Reading

Everything lives under `docs/`:

- `docs/v1/` — the basics track (start-small rule, minimal infra, the v1 phases).
- `docs/v2/` — architecture, local dev, Kubernetes, observability, Terraform,
  Ansible, CI/CD, distributed patterns, and databases (`00`–`09`).

## Ports on this VPS

Chosen to avoid the services already running here (Postgres 5432, Cockpit 9090,
Coolify/Portainer, an existing Grafana stack):

| Thing | Host port |
| ----- | --------- |
| Postgres | 55432 |
| Redis | 6379 |
| Redpanda (Kafka) | 9092 |
| employee / leave / reporting | 50051 / 50052 / 50053 |
| gateway (REST) | 8080 |
| kind ingress | 8088 |
| Grafana / Prometheus / Loki / Tempo | 3300 / 9099 / 3101 / 3201 |

## Toolchain (already installed)

Go 1.27.2, sqlc 1.31.1, buf 1.73.0, protoc-gen-go(-grpc), kubectl, kind, helm.

## Start now

```bash
# read
less docs/v1/README.md docs/v1/LEARNING_PATH.md
# begin
go mod init github.com/aymane-belassiria/hr-platform
make dev-up        # once you've written the minimal compose
```
