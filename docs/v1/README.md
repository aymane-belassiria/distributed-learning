# v1 · Basics — get the HR slice running locally

This is the **small track**. You build the smallest version of the HR platform
that actually runs end to end on your machine: four Go services, Postgres, Redis,
and a Kafka-compatible broker (Redpanda). Nothing cloud, nothing Kubernetes.

## How to use it

1. Read **[LEARNING_PATH.md](LEARNING_PATH.md)** — the phase list. Start at v1.0.
2. Follow the **start-small rule**: add a piece of config or a container **only
   when the current phase needs it**. Never copy the full stack up front.
3. Each phase tells you what to **add**, how to **know it works**, and what you
   are **deferring** to v2.
4. Write your own config, code and tests. This folder explains and lists tasks;
   it does not give you the implementation.

## What you'll have when v1 is done

```
client → gateway (:8080) → leave (:50052) → employee (:50051)
              │                 │
              └─ Redis cache    └─ outbox → Redpanda → reporting (:50053)
```

- **employee** — sqlc + Postgres, gRPC.
- **leave** — leave-request validation (overlap, working days, balance, notice)
  and approvals; calls `employee` over gRPC.
- **reporting** — a read model built from Kafka events, served from Redis.
- **gateway** — REST BFF with cache-aside.

## The ports

| Thing | Host port |
| ----- | --------- |
| Postgres | 55432 |
| Redis | 6379 |
| Redpanda (Kafka) | 9092 |
| employee | 50051 |
| leave | 50052 |
| reporting | 50053 |
| gateway (REST) | 8080 |

> 55432 and the remapped observability ports exist because this VPS already runs
> Postgres on 5432 and other services. Keep them.

## After v1

Move to **[v2](../v2/LEARNING_PATH.md)** for observability, resilience,
distributed databases, Kubernetes, Terraform, Ansible, CI/CD, the React UI, and
the final assembled project.
