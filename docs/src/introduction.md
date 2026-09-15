# Introduction

**minienv** creates effortless remote mini environments generated directly from
your docker compose config.

## The problem

You already describe your stack in `docker-compose.yml`. Getting that same stack
onto a shared cluster — so a teammate can click a link and review your branch —
normally means maintaining a second description of it: Kubernetes manifests, or
a Helm chart, that drifts from compose the moment anyone adds a service.

minienv removes the second description. It reads the compose file you already
have, plus a small extension block, and deploys it.

## What it does

Given a compose file, `minienv deploy`:

1. Builds and pushes images for any service with a `build:` section.
2. Prepares the target — creating the Kubernetes namespace, or the remote
   directory the compose project will live in.
3. Deploys your services, respecting `depends_on` order.
4. Optionally exposes a service publicly via ngrok, so it can be reviewed from
   anywhere.

`minienv destroy` tears the whole thing down again.

A complete, working config can be this small:

```yaml
x-minienv:
  k8s:
    context: minikube
    namespace: my-feature-branch

services:
  hello:
    image: rgonnella/demo-hello:latest
    ports:
      - "8080:8080"
```

Swap the `k8s` block for a `docker` one and the same compose file deploys to a
remote host instead:

```yaml
x-minienv:
  docker:
    namespace: my-feature-branch
    transport:
      ssh:
        host: dev-box.example.com
        identity: ~/.ssh/id_ed25519
```

## Where to go next

- [Getting Started](./getting-started.md) — install it and deploy something.
- [Configuration](./configuration.md) — the extension fields, and which compose
  fields minienv reads.
- [Configuration Reference](./configuration-reference.md) — every field, in
  tables.

> **Two targets, one at a time.** `k8s` deploys each service as a Helm release
> to a cluster; `docker` deploys the whole project to a single remote host. See
> [Deployers](./deployers.md).
