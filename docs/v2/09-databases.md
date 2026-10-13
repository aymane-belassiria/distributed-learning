# 09 · Databases: indexes, query debugging, and going distributed

SQL is the part everyone assumes they know and nobody debugs. This doc is the
missing layer between "the query works" and "the query is fast, correct under
concurrency, and still holds when the data is big and the database is no longer
one machine."

Everything here is applied to the HR schema so the exercises are real.

> Tooling assumption: Postgres 16+, `psql`, and the app's three databases
> (`employee`, `leave`, `reporting`). Add extensions with
> `CREATE EXTENSION IF NOT EXISTS ...;`.

---

## Part A · Reading a query plan (the core skill)

### A1. Turn on the instruments

```sql
-- statement stats (reset with: SELECT pg_stat_statements_reset();)
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;

-- log anything slower than 50ms
ALTER SYSTEM SET log_min_duration_statement = '50ms';
-- log the plan of anything slower than 200ms
CREATE EXTENSION IF NOT EXISTS auto_explain;
ALTER SYSTEM SET auto_explain.log_min_duration = '200ms';
ALTER SYSTEM SET auto_explain.log_analyze    = on;
ALTER SYSTEM SET auto_explain.log_buffers    = on;
SELECT pg_reload_conf();
```

Find the worst offenders:

```sql
SELECT calls,
       round(mean_exec_time::numeric, 2) AS mean_ms,
       round(total_exec_time::numeric, 1) AS total_ms,
       rows,
       shared_blks_read,
       left(query, 80) AS query
FROM pg_stat_statements
ORDER BY total_exec_time DESC
LIMIT 10;
```

`pg_stat_statements` normalizes queries (literals → `$1`), so the same shape
aggregates together. `shared_blks_read` high = cache misses = probably a missing
index. `calls` high + `mean_ms` low = maybe an N+1.

### A2. EXPLAIN vs EXPLAIN ANALYZE

```sql
EXPLAIN (ANALYZE, BUFFERS, VERBOSE)
SELECT id, employee_id, start_date, end_date
FROM leave_requests
WHERE employee_id = '...' AND status <> 'cancelled';
```

- `EXPLAIN` = **estimate** (no execution).
- `EXPLAIN ANALYZE` = **executes** (careful with writes; wrap in a rollback).
- `BUFFERS` = pages hit/read — the real I/O picture.
- `VERBOSE` = output columns.

### A3. The node types you must recognize

| Node | Means | Usually |
| ---- | ----- | ------- |
| `Seq Scan` | reads the whole table | fine for tiny tables, red flag for large ones with a selective filter |
| `Index Scan` | walks the index, fetches rows from the heap | good for selective lookups |
| `Index Only Scan` | answers from the index alone | best; needs `visibility map` (VACUUM) to be cheap |
| `Bitmap Heap Scan` | builds a bitmap of matching pages then reads them | good for medium selectivity / many rows |
| `Nested Loop` | for each outer row, scan inner | great for few outer rows, catastrophic if the estimate is wrong |
| `Hash Join` / `Merge Join` | joins | right for larger sets |
| `Sort` / `HashAggregate` | sorting/aggregation | watch for `Sort Method: external merge Disk` (spills to disk) |
| `Limit` + `rows removed by filter` | early stop | high removal = wasted work |

The single most important number: **estimated rows vs actual rows**. A big gap
means the planner's statistics are stale or the query is un-sargable.

```sql
-- refresh stats after a big load
ANALYZE leave_requests;
-- how the planner sees the data
SELECT * FROM pg_stats WHERE tablename = 'leave_requests' AND attname = 'status';
```

### A4. Worked example — the overlap check

The rule "no two non-cancelled requests overlap" becomes:

```sql
SELECT 1
FROM leave_requests
WHERE employee_id = $1
  AND status <> 'cancelled'
  AND start_date <= $3   -- $3 = requested end_date
  AND end_date   >= $2   -- $2 = requested start_date
LIMIT 1;
```

Without an index this is a seq scan every time. Add a **partial, composite**
index that exactly matches the filter and the range:

```sql
CREATE INDEX idx_leave_overlap
  ON leave_requests (employee_id, start_date, end_date)
  WHERE status <> 'cancelled';
```

Re-run `EXPLAIN (ANALYZE)` and watch `Seq Scan` become an `Index Scan`. Then
**prove the index is used** by forcing the bad path:

```sql
SET enable_indexscan = off;
EXPLAIN ANALYZE SELECT ...;   -- back to Seq Scan, note the time
SET enable_indexscan = on;
```

### A5. The N+1 trap (sqlc does not save you)

```go
// ANTI-PATTERN: one query per employee
for _, id := range ids {
    emp, _ := q.GetEmployee(ctx, id) // N round-trips
}
```

Fix it with a single `WHERE id = ANY($1::uuid[])`, or a join in the read model.
Detect it with `pg_stat_statements.calls` exploding while `rows` stays low, or by
counting spans in Tempo (one `db.query` span per employee).

### A6. Connection pooling

`pgxpool` is the pool. Tune it and **measure** it:

```go
cfg, _ := pgxpool.ParseConfig(dsn)
cfg.MaxConns = 10            // per replica; budget = db_max_connections / replicas
cfg.MinConns = 2
cfg.MaxConnIdleTime = 5 * time.Minute
cfg.HealthCheckPeriod = 30 * time.Second
```

```sql
SELECT state, count(*) FROM pg_stat_activity GROUP BY state;
SELECT count(*) FROM pg_stat_activity WHERE backend_type = 'client backend';
```

Too many app replicas × too many `MaxConns` and you exhaust `max_connections`.
At scale you put **PgBouncer** in transaction mode in front — but read the fine
print: prepared statements and session-level state don't survive transaction
pooling. This is exactly the kind of trade-off worth a paragraph in your notes.

---

## Part B · Schema and index design

### B1. Index types and when each wins

| Index | Use it for | HR example |
| ----- | ---------- | ---------- |
| B-tree (default) | `=`, `<`, `>`, `BETWEEN`, `ORDER BY` | `employees(department_id, last_name)` |
| Composite | the **leftmost prefix** rule | `(employee_id, start_date)` serves `employee_id =` and ranges on `start_date` |
| Partial (`WHERE ...`) | a hot subset | only `status = 'requested'` rows |
| Covering (`INCLUDE (...)`) | index-only scans | `(email) INCLUDE (id)` for auth lookups |
| GIN | `jsonb`, arrays, full text | `employees(skills jsonb)`, `to_tsvector(name)` |
| GiST + `btree_gist` | ranges, exclusion constraints | the no-overlap guarantee (below) |

**Leftmost-prefix rule:** an index on `(a, b, c)` helps `a`, `a,b`, `a,b,c` — but
not `b` alone. Put equality columns first, range columns last.

**Write amplification:** every index slows `INSERT/UPDATE/DELETE`. Index what you
filter and sort on; delete indexes you can't justify with a query.

### B2. Let the database enforce the rule

Application validation can race. A stronger guarantee for "no overlap" is an
**exclusion constraint**:

```sql
CREATE EXTENSION IF NOT EXISTS btree_gist;

ALTER TABLE leave_requests
  ADD CONSTRAINT leave_no_overlap
  EXCLUDE USING gist (
    employee_id WITH =,
    daterange(start_date, end_date, '[]') WITH &&
  )
  WHERE (status <> 'cancelled');
```

Now two concurrent inserts can't both win — the second gets a `23P01`
(`exclusion_violation`). Map that to a `FailedPrecondition`/409 in your service.
Other constraints worth adding: `CHECK (start_date <= end_date)`, `CHECK (status
IN (...))`, `CHECK (working_days > 0)`, and a `UNIQUE(employee_id, email)`.

### B3. Partitioning (single-node scaling)

`leave_requests` grows forever. Partition by year:

```sql
CREATE TABLE leave_requests (
  id uuid NOT NULL,
  employee_id uuid NOT NULL,
  start_date date NOT NULL,
  ...
) PARTITION BY RANGE (start_date);

CREATE TABLE leave_requests_2025 PARTITION OF leave_requests
  FOR VALUES FROM ('2025-01-01') TO ('2026-01-01');
CREATE TABLE leave_requests_2026 PARTITION OF leave_requests
  FOR VALUES FROM ('2026-01-01') TO ('2027-01-01');
```

Benefits: partition pruning (queries with a date filter touch one child), cheap
bulk deletes (`DROP TABLE leave_requests_2024`), smaller indexes. Gotcha: unique
and exclusion constraints must **include the partition key**, so the overlap
constraint above becomes harder if you partition — a real trade-off to document.

---

## Part C · Transactions and concurrency

### C1. What MVCC gives you

Postgres uses MVCC: readers don't block writers. The default isolation is **Read
Committed**. Know the ladder:

| Level | Prevents | Doesn't prevent |
| ----- | -------- | --------------- |
| Read Committed | dirty reads | non-repeatable reads, phantoms, lost updates |
| Repeatable Read (snapshot here) | non-repeatable reads | phantoms (mostly), write skew |
| Serializable (SSI here) | write skew / anomalies | — (but you get `40001` retries) |

### C2. The approval race, three ways

Two managers approve the same request. Pick your defense:

**Optimistic status guard** (simplest, already in v1.5):

```sql
UPDATE leave_requests
SET status = $2, decided_at = now(), decided_by = $3
WHERE id = $1 AND status = 'requested';
-- rows affected = 0  -> someone else already decided -> 409
```

**Pessimistic row lock** for the balance update:

```sql
SELECT entitled_days, used_days
FROM leave_balances
WHERE employee_id = $1 AND leave_type = $2 AND year = $3
FOR UPDATE;   -- holds the row until commit; serializes concurrent approvals

UPDATE leave_balances
SET used_days = used_days + $4
WHERE employee_id = $1 AND leave_type = $2 AND year = $3;
```

`FOR UPDATE` prevents lost updates when two approvals touch the same balance.
Keep the lock scope tiny (lock late, commit fast) so you don't serialize the
whole table.

**SERIALIZABLE + retry** (highest level, needs retry logic):

```sql
BEGIN ISOLATION LEVEL SERIALIZABLE;
...
COMMIT;  -- may fail with SQLSTATE 40001 (serialization_failure)
```

Write a small `withRetry(ctx, txFunc, isSerializationFailure)` helper and unit
test it by making the first attempt return a synthetic `40001`.

### C3. Deadlocks and lock ordering

A deadlock happens when two transactions grab the same locks in opposite order.
`UPDATE ... WHERE employee_id = ANY(...)` touching multiple balance rows is the
classic. Prevention: **always acquire locks in a deterministic order** (e.g.
`ORDER BY employee_id, leave_type`). Postgres detects deadlocks and kills one with
`40P01`; log it, don't crash the process.

### C4. Advisory locks (coordination without a lock table)

For a singleton job (e.g. monthly leave accrual) across replicas:

```sql
SELECT pg_try_advisory_lock(42);   -- true = you own the job this run
-- ... do the accrual ...
SELECT pg_advisory_unlock(42);
```

Session-scoped, so it releases when the connection dies. Simpler than introducing
Redis for one job — the choice is itself a lesson.

---

## Part D · Going distributed

This is where "a database" becomes "a distributed system."

### D1. Replication basics

- **Physical (streaming) replication**: byte-for-byte WAL shipping to a hot
  standby. All-or-nothing, whole cluster.
- **Logical replication**: publish/subscribe per table; lets the `reporting`
  service subscribe to `leave`'s changes directly.
- **Read replicas**: offload analytics/reporting reads.

```sql
-- on the primary
SELECT client_addr, state, sent_lsn, replay_lsn,
       replay_lag FROM pg_stat_replication;

-- logical: publish on the writer, subscribe on the reader
CREATE PUBLICATION leave_pub FOR TABLE leave_requests;
CREATE SUBSCRIPTION leave_sub CONNECTION '...' PUBLICATION leave_pub;
```

**The catch you must internalize — replication lag and read-your-writes:** after
approving a request, an immediate read from a replica may not see it yet. Real
systems solve this by (a) reading your own writes from the primary, (b) pinning a
session to a replica until it catches up (LSN tracking), or (c) accepting stale
reads and designing the UI for it. Document your choice.

### D2. Partitioning vs sharding

- **Partitioning** = one logical database, many physical tables. Cheap, no app
  change (mostly).
- **Sharding** = many independent databases. Required when one machine can no
  longer hold the write volume. You now own: shard key choice, cross-shard
  queries, rebalancing, distributed transactions.

Natural shard key here: **`employee_id`** (or `department_id`). Everything about an
employee lands on one shard, so the overlap/balance invariant stays local — no
cross-shard transaction. That is the single most important sharding insight:
**choose a shard key so your invariants are single-shard.**

### D3. CAP / PACELC in one paragraph

Under a network **P**artition you choose **C**onsistency or **A**vailability
(CAP). Even with no partition (**E**lse) you choose **L**atency or
**C**onsistency (PACELC). Postgres with sync replicas = CP-ish (writes block until
replicated) and higher latency. Redis counters = AP-ish (fast, maybe stale) —
which is exactly why the reporting read model tolerates staleness but the balance
update does not.

### D4. Distributed transactions: 2PC vs Saga

There is no free lunch joining two databases atomically:

- **Two-phase commit (2PC)**: a coordinator prepares all participants, then
  commits. Correct but blocking — if the coordinator dies between phases,
  participants hold locks. Rarely worth it across services.
- **Saga**: a sequence of local transactions with **compensating actions** on
  failure. E.g. "approve leave" = (1) mark request approved, (2) increment balance,
  (3) notify — and compensate by re-emitting a `leave.reverted` if step 3's
  downstream rejects.

Your **transactional outbox is the building block of a saga**: the local tx +
intent-to-publish is atomic, the relay makes delivery happen eventually. You
already built this in v1.4 — now you can name it.

### D5. Distributed locks and idempotency

- **Redis lock**: `SET lock:accrual <token> NX PX 30000` then release with a
  Lua compare-and-delete so you never delete someone else's lock. (Note the
  well-known Redlock critique: the algorithm is not safe under clock skew and
  GC pauses over independent Redis masters. For one job, advisory locks or a DB
  row are usually the honest answer.)
- **Idempotency keys**: the DynamoDB table from v2.5 stores
  `(request_id → result)` so a retried approval returns the first result instead
  of double-applying. Same idea as a `UNIQUE` constraint on `leave_requests(id)`.

### D6. Distributed SQL (NewSQL) — know they exist

CockroachDB, YugabyteDB, Google Spanner, Vitess: they shard + replicate
automatically and offer serializable distributed transactions, at the cost of
more latency per write and operational complexity. The HR system is deliberately
small — but you should be able to say *when* you'd reach for one (multi-region,
write volume beyond one primary, strict global consistency).

---

## Part E · Exercises (do these against your running HR platform)

1. **Baseline then fix.** Run the overlap query with `EXPLAIN (ANALYZE,
   BUFFERS)` on an empty-ish table, insert 100k `leave_requests` with a
   `generate_series`, re-run, add `idx_leave_overlap`, re-run. Save all three
   plans in `docs/notes/`.
2. **Find the regression.** Use `SET enable_indexscan = off` to simulate a bad
   plan, record the timing, and explain which node got worse.
3. **Make the DB enforce the rule.** Add the `btree_gist` exclusion constraint
   from Part B2. Write an integration test (v1.5) that fires two overlapping
   inserts concurrently and asserts one fails with `23P01`.
4. **Instrument.** Turn on `pg_stat_statements` + `auto_explain`, exercise the API
   with `hey`/`k6`, and list the top 5 queries by `total_exec_time`.
5. **Kill a query.** Start `SELECT pg_sleep(120)` and terminate it from another
   session with `pg_cancel_backend` vs `pg_terminate_backend`; note the difference.
6. **The approval race.** Write a test with two goroutines calling
   `DecideLeaveRequest` on the same id; assert exactly one succeeds and
   `used_days` moved once. Then repeat with `FOR UPDATE` and with `SERIALIZABLE`
   + retry and compare.
7. **Reproduce a deadlock.** Two transactions updating two balance rows in
   opposite order; observe `40P01`, then fix by ordering the locks.
8. **Partition.** Convert `leave_requests` to yearly partitions, confirm partition
   pruning with `EXPLAIN`, and show that a 2024 drop is instant.
9. **Replicate.** Stand up a second Postgres as a replica, subscribe `reporting`
   to logical changes, and measure `replay_lag` under write load. Demonstrate the
   stale-read window.
10. **Write it down.** A `docs/notes/database-decisions.md` recording your shard
    key, your isolation choices, and where you accept staleness.

---

## Cheat sheet

```sql
EXPLAIN (ANALYZE, BUFFERS, VERBOSE) <query>;
ANALYZE <table>;
SET enable_indexscan = off|on;

SELECT * FROM pg_stat_statements ORDER BY total_exec_time DESC LIMIT 10;
SELECT * FROM pg_stat_activity WHERE state <> 'idle';
SELECT client_addr, replay_lag FROM pg_stat_replication;

SELECT pg_try_advisory_lock(42);
SELECT pg_cancel_backend(pid);      -- graceful
SELECT pg_terminate_backend(pid);   -- hard
```

```bash
psql "$LEAVE_DATABASE_URL" -c '\di+'          # indexes
psql "$LEAVE_DATABASE_URL" -c '\d+ leave_requests'   # columns, constraints, indexes
pgbench -c 20 -j 4 -T 60 "$LEAVE_DATABASE_URL"       # load
```

> Rule of thumb: measure before you optimize, write the query you can read,
> index the query you actually run, and let the database enforce the invariant
> you can't afford to violate.
