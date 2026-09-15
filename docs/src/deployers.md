# Deployers

The `x-minienv` block is keyed by target: configure exactly one. Pick the one
whose prerequisites you already have.

## Kubernetes

Deploys each compose service as its own Helm release. There is no chart to
maintain in your repository.

Needs a reachable Kubernetes context:

```yaml
x-minienv:
  k8s:
    context: minikube
    namespace: my-branch
```

Per-service overrides go under `x-minienv-k8s-service`.

## Docker

Deploys the whole project to a single remote host as one `docker compose`
project, over a transport.

Needs an SSH-reachable host running `docker` and its compose plugin:

```yaml
x-minienv:
  docker:
    namespace: my-branch
    transport:
      ssh:
        host: dev-box.example.com
        identity: ~/.ssh/id_ed25519
```

Per-service overrides go under `x-minienv-docker-service`.

Every field of both blocks is listed in
[Configuration Reference](./configuration-reference.md#x-minienv).
