# 07 · CI/CD

Two GitHub Actions workflows you will build:

- `.github/workflows/ci.yml` — runs on every push and PR.
- `.github/workflows/cd.yml` — builds/pushes images and deploys on `main`.

## CI jobs

| Job | What it does |
| --- | ------------ |
| `go` | `go build`, `go vet`, `go test -race` |
| `lint` | `golangci-lint` (config in `.golangci.yml`) |
| `codegen` | runs `buf generate` + `sqlc generate`, fails on drift in `gen/` and generated DB code |
| `integration` | `go test -tags integration` with Testcontainers (Postgres, Redpanda) |
| `docker` | builds each service image with Buildx + GHA cache (matrix: employee, leave, reporting, gateway) |
| `terraform` | `terraform fmt -check` and `validate` for both modules |
| `ansible` | `--syntax-check` both playbooks |
| `k8s` | `kubectl kustomize` both overlays → `kubeconform -strict` |
| `web` | `npm ci && npm run build` |

The **codegen** job keeps the repo honest: generated code is committed, so CI
proves it matches the `.proto`/`.sql` sources.

## CD jobs

1. `images` — matrix over the four services, pushes to **GHCR**
   (`ghcr.io/<owner>/hr-platform/<service>`), tagged with the short SHA, git tag
   and `latest` on `main`.
2. `web` — same for the React image.
3. `deploy` — runs on a **self-hosted runner** on the VPS. It rewrites the
   Kustomize image tags to the commit SHA and applies the dev overlay.

### Registering the runner

```bash
# On the VPS (Repository → Settings → Actions → Runners → New self-hosted runner)
mkdir actions-runner && cd actions-runner
curl -o actions-runner.tar.gz -L <url-from-github>
tar xzf actions-runner.tar.gz
./config.sh --url https://github.com/<you>/hr-platform --token <token> --labels hr-platform
./run.sh          # or install as a systemd service
```

The deploy job targets `runs-on: [self-hosted, hr-platform]`.

### No self-hosted runner? Deploy over SSH

```yaml
- name: Deploy over SSH
  uses: appleboy/ssh-action@v1
  with:
    host: ${{ secrets.VPS_HOST }}
    username: root
    key: ${{ secrets.VPS_SSH_KEY }}
    script: |
      cd /srv/hr-platform
      git pull
      kubectl apply -k deploy/k8s/overlays/dev
      kubectl -n hr rollout status deploy/gateway
```

## Local pre-flight (mirror CI)

```bash
make generate
gofmt -l . ; go vet ./...
golangci-lint run
go test -race ./...
terraform fmt -check -recursive infra/terraform
kubectl kustomize deploy/k8s/overlays/dev | kubectl apply --dry-run=client -f -
cd web && npm ci && npm run build
```

## Ideas to extend

- Add a `trivy`/`grype` image-scan job.
- Sign images with cosign and verify on deploy.
- Gate `deploy` behind a GitHub Environment requiring manual approval.
