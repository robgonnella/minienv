# Introduction

**minienv** creates effortless remote mini environments generated directly from
your docker compose config.

## The problem

You already describe your stack in `docker-compose.yml`. Getting that same stack
onto a shared cluster — so a teammate can click a link and review your branch —
normally means maintaining a second description of it: a set of Kubernetes
manifests, or a Helm chart, that drifts from compose the moment anyone adds a
service.

minienv removes the second description. It reads the compose file you already
have, plus a small extension block, and deploys it.

## What it does

Given a compose file, `minienv deploy`:

1. Builds and pushes images for any service with a `build:` section.
2. Creates the target namespace if it does not exist.
3. Deploys one release per compose service, in `depends_on` order.
4. Optionally exposes a service publicly via ngrok, so it can be reviewed from
   anywhere.

`minienv destroy` tears the whole thing down in reverse order.

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

## Where to go next

- [Getting Started](./getting-started.md) — install it and deploy something.
- [Configuration](./configuration.md) — the extension fields, and which compose
  fields minienv reads.
- [Configuration Reference](./configuration-reference.md) — every field, in
  tables.

> **Kubernetes is the only deployment target today.** More are planned. See
> [Deployers](./deployers.md).
