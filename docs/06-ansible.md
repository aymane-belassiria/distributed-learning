# 06 · Ansible

Ansible turns a fresh VPS into a machine that can build and run this platform.
The playbooks are **idempotent** — run them repeatedly and after the first pass
you should see `changed=0`.

## Layout you will build

```
infra/ansible/
  ansible.cfg
  inventory.ini
  playbooks/
    bootstrap.yml     # OS packages, Docker, Go, dev/k8s toolchain
    deploy.yml        # kind cluster + build + apply
  roles/
    docker/           # install + enable Docker, compose plugin
    golang/           # Go toolchain to /usr/local/go
    devtools/         # sqlc, buf, protoc plugins, kubectl, kind, helm, golangci-lint
    kind/             # create the cluster, addons, /etc/hosts
```

## Inventory

Default to a **local** connection (this machine). For a real remote VPS:

```ini
[vps]
hr-vps ansible_host=203.0.113.10 ansible_user=root \
  ansible_ssh_private_key_file=~/.ssh/id_ed25519
```

## Bootstrap

```bash
cd infra/ansible
ansible-playbook -i inventory.ini playbooks/bootstrap.yml

# idempotency check:
ansible-playbook -i inventory.ini playbooks/bootstrap.yml   # expect changed=0
```

## Deploy

`playbooks/deploy.yml` ensures the kind cluster exists, installs addons, then
builds/loads images and applies the dev overlay.

```bash
ansible-playbook -i inventory.ini playbooks/deploy.yml
```

## Idempotency patterns to use

| Pattern | Where |
| ------- | ----- |
| `creates:` guard on commands | `devtools` (protoc plugins, kubectl, helm) |
| `get_url` + `changed_when` | `devtools` (sqlc, buf, kind) |
| version check before install | `golang` (`go version` contains target) |
| `systemd` state+enabled | `docker` |
| `lineinfile` for `/etc/hosts` | `kind` (hr.localhost, grafana.hr.localhost) |

## What to try next

- Add a `templates/hosts.j2` to generate `/etc/hosts` from a variable.
- Add `handlers` (e.g. restart Docker when its config changes).
- Use `ansible-vault` for secrets instead of a committed Secret.
- Replace raw `command` deploy steps with the `kubernetes.core.k8s` module.
- Run `ansible-lint` in CI.
