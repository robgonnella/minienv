# Deployers

The `x-minienv` block is keyed by target: configure exactly one. Configuring
more than one is an error, not a silent choice between them.

Pick the one whose prerequisites you already have.

## Kubernetes

Deploys each compose service as its own Helm release, using a chart minienv
generates internally — there is no chart to maintain in your repository, and no
`helm` binary to install. Releases go up in `depends_on` order and come down in
reverse.

It activates when you set both required fields:

```yaml
x-minienv:
  k8s:
    context: minikube
    namespace: my-branch
```

Per-service overrides live under `x-minienv-k8s-service`.

## Docker

Deploys the whole project to a single remote host over a transport, as one
`docker compose` project living in `~/.minienv/<namespace>`.

It activates when you configure a namespace and exactly one transport:

```yaml
x-minienv:
  docker:
    namespace: my-branch
    ssh:
      host: dev-box.example.com
      user: me
      identity: ~/.ssh/id_ed25519
```

Per-service overrides live under `x-minienv-docker-service`.

`namespace` is what keeps two developers off each other's environments on one
shared host, so give each person their own — it names both the compose project
and its directory.

The `docker` block is keyed by transport: configure exactly one. Every
transport's fields are listed in
[Configuration Reference](./configuration-reference.md#docker).

### What teardown removes

`minienv destroy` takes the project down, volumes included, and removes
`~/.minienv/<namespace>` entirely. The directory is removed even if the teardown
itself fails, so a retry starts clean; the command still exits non-zero in that
case, so you will see the failure.
