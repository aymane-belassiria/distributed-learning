# 05 · Terraform → Floci (local AWS)

[Floci](https://github.com/floci-io/floci) is a free, local AWS emulator on
`http://localhost:4566` — a drop-in LocalStack replacement. Point Terraform, the
AWS CLI or an SDK at it and everything behaves like AWS, without an account.

## 1. Start Floci

```bash
docker compose --profile floci up -d floci
curl -s http://localhost:4566/_floci/health   # or open /_floci/ui
```

The container mounts `/var/run/docker.sock`, so Docker-backed services (RDS,
ElastiCache, EKS/k3s, …) work too.

## 2. Provision local AWS resources

`infra/terraform/floci/main.tf` creates resources that map to real HR needs:

- **S3** bucket — archive of leave **attachments** / decision PDFs, with versioning.
- **SQS** queue + **dead-letter queue** — an alternative async path for approvals
  (and a place for events that fail validation downstream).
- **SNS** topic — manager **notifications** ("your report requested leave").
- **DynamoDB** table — **approval idempotency** keys (`pk`/`sk`, TTL) so a
  double-clicked approval cannot double-apply.
- **ECR** repository — the service images.

```bash
cd infra/terraform/floci
terraform init
terraform plan
terraform apply -auto-approve
terraform output
```

### Why this provider block works

```hcl
provider "aws" {
  region                      = "us-east-1"
  access_key                  = "test"          # any non-empty values
  secret_key                  = "test"
  skip_credentials_validation = true
  skip_metadata_api_check     = true
  skip_requesting_account_id  = true
  s3_use_path_style           = true
  endpoints {
    s3       = "http://localhost:4566"
    # ... sqs, sns, dynamodb, ecr, iam, sts
  }
}
```

Remove the `endpoints` block and supply real credentials and **the same config
targets real AWS** — that portability is the point.

## 3. Verify with the AWS CLI

```bash
export AWS_ENDPOINT_URL=http://localhost:4566 AWS_ACCESS_KEY_ID=test AWS_SECRET_ACCESS_KEY=test AWS_DEFAULT_REGION=us-east-1
aws s3 ls
aws sqs list-queues
aws dynamodb list-tables
aws sns list-topics
aws ecr describe-repositories
```

## 4. EKS inside Floci (the interesting one)

Floci emulates EKS by running a **real k3s** API server in a container, so you get
an actual Kubernetes control plane from Terraform.

```bash
cd infra/terraform/eks-floci && terraform init && terraform apply
aws eks update-kubeconfig --name hr-platform --endpoint-url http://localhost:4566
kubectl get nodes
```

> EKS needs a lot of memory (k3s + the emulator). On a small VPS run EKS **or**
> kind, not both at once.

## 5. Tie it back to the app (exercise)

The outputs give you a bucket, a queue and a table. Extend the platform so that:
1. `LeaveService.DecideLeaveRequest` writes a decision PDF to S3.
2. Approval notifications are published to the SNS topic → SQS.
3. Approvals use the DynamoDB table for an idempotency key, so a retried call is a
   no-op.

All of these point at Floci locally and at real AWS in production — no code change
beyond configuration.

## Files you will build

| File | Purpose |
| ---- | ------- |
| `infra/terraform/floci/versions.tf` | provider constraints |
| `infra/terraform/floci/main.tf`     | resources |
| `infra/terraform/floci/variables.tf`| region/endpoint/prefix |
| `infra/terraform/floci/outputs.tf`  | endpoint, ARNs, URLs |
| `infra/terraform/eks-floci/main.tf` | EKS cluster + node group |

## Clean up

```bash
cd infra/terraform/floci && terraform destroy
docker compose --profile floci down
```
