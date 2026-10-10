# 03 · Kubernetes with kind

We run the whole HR platform on **kind** because it is fast, cheap and behaves
like a real cluster. Later (`docs/05`) Terraform stands up an EKS control plane
inside Floci — the same manifests apply.

## 1. Create the cluster

```bash
kind create cluster --config deploy/kind/kind-config.yaml
kubectl cluster-info --context kind-hr-platform
```

Use a single node (memory-friendly) and publish the ingress controller on host
port **8088** (80/443 are taken on this host).

## 2. Install addons

- **ingress-nginx** — routes web/gateway/grafana.
- **metrics-server** — required by the HPAs (patched with `--kubelet-insecure-tls`).

```bash
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/kind/deploy.yaml
kubectl wait --namespace ingress-nginx --for=condition=ready pod \
  --selector=app.kubernetes.io/component=controller --timeout=180s
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
kubectl patch deployment metrics-server -n kube-system --type=json \
  -p '[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--kubelet-insecure-tls"}]'

echo "127.0.0.1 hr.localhost grafana.hr.localhost" | sudo tee -a /etc/hosts
```

## 3. Build and load images

kind runs its own containerd, so host images must be loaded into it:

```bash
docker build -f services/employee/Dockerfile  -t hr/employee:dev  .
docker build -f services/leave/Dockerfile     -t hr/leave:dev     .
docker build -f services/reporting/Dockerfile -t hr/reporting:dev .
docker build -f services/gateway/Dockerfile   -t hr/gateway:dev   .
docker build -t hr/web:dev ./web

for img in employee leave reporting gateway web; do
  kind load docker-image hr/$img:dev --name hr-platform
done
```

## 4. Deploy (Kustomize)

```
deploy/k8s/base/            # environment-agnostic
deploy/k8s/overlays/dev/    # local images, debug logging, 1 replica
deploy/k8s/overlays/prod/   # pinned tags, 2 replicas
```

```bash
kubectl apply -k deploy/k8s/overlays/dev
kubectl -n hr get all
kubectl -n hr get pods -w
```

Created: namespace, ConfigMap/Secret, Postgres StatefulSet (PVC + init SQL for the
three databases), Redis, Redpanda, otel-lgtm, the four services, the web
Deployment, Ingress and HPAs.

The ConfigMap supplies each service its own `DATABASE_URL` plus the in-cluster
DNS names (`employee:50051`, `leave:50052`, `reporting:50053`, `redis:6379`,
`redpanda:9092`, `otel-lgtm:4317`).

## 5. Access

- App:     http://hr.localhost:8088
- Grafana: http://grafana.hr.localhost:8088 (admin/admin)
- API:     http://hr.localhost:8088/api/v1/employees

## 6. Operate it

```bash
kubectl -n hr logs deploy/gateway -f                 # structured JSON + trace_id
kubectl -n hr set image deploy/leave leave=hr/leave:dev
kubectl -n hr rollout status deploy/leave
kubectl -n hr rollout undo deploy/leave
kubectl -n hr scale deploy/gateway --replicas=3
kubectl -n hr get hpa -w
```

> Images are **distroless** (no shell, non-root). Use logs/`kubectl debug` rather
> than `exec sh`.

## 7. Preview what will be applied

```bash
kubectl kustomize deploy/k8s/overlays/dev | less
kubectl apply -k deploy/k8s/overlays/dev --dry-run=server
```

## Memory notes for a small VPS

Everything fits in ~2–3 GB. If you are tight, drop the in-cluster `otel-lgtm` and
point services at the host LGTM instead (an `ExternalName` Service or a NodePort —
good exercise).

## What to try next

- Add a `NetworkPolicy` letting only the gateway reach service ports; `leave`
  reaching `employee`.
- Add a Kubernetes **CronJob** for monthly leave accrual.
- Convert the base into a Helm chart with values per environment.
- Canary the `gateway` with two Deployments + weighted ingress annotations.
