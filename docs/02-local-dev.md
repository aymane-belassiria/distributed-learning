# 02 · Local development

## Prerequisites

| Tool            | Version   | Install |
| --------------- | --------- | ------- |
| Docker + Compose| v2        | https://docs.docker.com/engine/install/ |
| Go              | 1.24+     | https://go.dev/dl/ |
| make            | any       | `apt-get install make` |
| sqlc            | 1.31+     | `go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest` |
| buf             | 1.73+     | https://github.com/bufbuild/buf/releases |
| protoc plugins  | latest    | `go install google.golang.org/protobuf/cmd/protoc-gen-go@latest && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest` |
| Node            | 20+       | https://nodejs.org |
| grpcurl (opt)   | latest    | `go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest` |

## 1. Start infrastructure

```bash
cp .env.example .env
make dev-up
docker compose ps
```

You should see `postgres`, `redis`, `redpanda` and the observability stack
(`otel-lgtm`) healthy. Grafana is at http://localhost:3300 (admin/admin).

## 2. Generate code

```bash
make generate      # buf generate && sqlc generate
```

## 3. Run the services

Four terminals:

```bash
make run-employee     # :50051
make run-leave        # :50052
make run-reporting    # :50053
make run-gateway      # :8080
```

Each service runs its own migrations at startup (goose + embedded FS).

Health checks:

```bash
curl localhost:8080/healthz
curl localhost:8080/readyz     # pings Redis
```

## 4. Seed HR data and exercise the flow

Create a department and employees, then submit a leave request:

```bash
# department + employee
curl -s -XPOST localhost:8080/api/v1/employees -H 'Content-Type: application/json' -d '{
  "first_name":"Amine","last_name":"Idrissi","email":"amine@example.ma",
  "department_id":"<uuid>","employment_type":"full_time"
}'

# submit leave (valid)
curl -s -XPOST localhost:8080/api/v1/leave-requests -H 'Content-Type: application/json' -d '{
  "employee_id":"<uuid>","leave_type":"annual",
  "start_date":"2026-07-01","end_date":"2026-07-10","reason":"summer"
}'

# submit leave (overlapping) -> expect 4xx with reasons
curl -s -XPOST localhost:8080/api/v1/leave-requests -H 'Content-Type: application/json' -d '{
  "employee_id":"<uuid>","leave_type":"annual",
  "start_date":"2026-07-05","end_date":"2026-07-06"
}'

# decide it
curl -s -XPOST localhost:8080/api/v1/leave-requests/<id>/decide -d '{"approve":true,"decided_by":"<manager-uuid>"}'

# read models
curl -s localhost:8080/api/v1/employees/<id>/balance
curl -s localhost:8080/api/v1/reports/departments

# cache behaviour
curl -sD- -o/dev/null localhost:8080/api/v1/employees | grep -i x-cache   # MISS
curl -sD- -o/dev/null localhost:8080/api/v1/employees | grep -i x-cache   # HIT
```

## 5. gRPC directly

```bash
grpcurl -plaintext localhost:50051 list
grpcurl -plaintext -d '{"page_size":5}' localhost:50051 hr.v1.EmployeeService/ListEmployees
grpcurl -plaintext -d '{"employee_id":"<uuid>"}' localhost:50053 hr.v1.ReportingService/GetDepartmentSummary
```

## Tests

```bash
make test              # unit tests (table-driven leave validation lives here)
make test-integration  # Testcontainers (Postgres, Redpanda)
make test-all
```

## Database access

```bash
docker exec -it <pg-container> psql -U hr -d leave
# \dt, select * from leave_requests; select * from leave_balances; select count(*) from outbox;
docker exec -it <pg-container> psql -U hr -d employee -c 'select employee_code,status from employees;'
```

## Kafka

```bash
# Redpanda console: http://localhost:8085
docker exec -it <redpanda-container> rpk topic list
docker exec -it <redpanda-container> rpk topic consume leave.requested
docker exec -it <redpanda-container> rpk group describe reporting
```

## Troubleshooting

| Symptom | Fix |
| ------- | --- |
| `bind: address already in use` on 55432 | another Postgres uses the port; change the mapping |
| Overlap not detected | make sure the overlap query excludes `cancelled`/`rejected` |
| Balance never updates | check the consumer + the status guard on `DecideLeaveRequest` |
| `X-Cache` never HITs | confirm `REDIS_ADDR` and that you hit the gateway |
| No traces in Tempo | confirm `OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317` |
| Port 9090 conflict | Grafana/Prometheus are remapped (3300/9099); Cockpit owns 9090 |

## Stop everything

```bash
make dev-down
pkill -x employee; pkill -x leave; pkill -x reporting; pkill -x gateway
```
