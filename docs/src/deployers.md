# Deployers

minienv separates _reading your compose config_ from _deploying it somewhere_.
The reading half is fixed; the deploying half is pluggable.

The `x-minienv` block is keyed by target: configure exactly one. Configuring
more than one is an error, not a silent choice between them.

Each target is documented on its own terms below; pick the one whose
prerequisites you already have.

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
`docker compose` project. minienv rewrites your compose config — dropping
skipped services, clearing builds, published ports and bind mounts, pinning
images to the tags it just pushed — writes it to `~/.minienv/<namespace>` on the
remote, and runs `docker compose up -d --remove-orphans` there.

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

`namespace` does double duty: it is the compose project name on the remote host
_and_ the directory the deployment lives in. compose derives volume, network
and container names from the project name, so it is what keeps two developers
off each other's environments on one shared host.

The `docker` block is keyed by transport, the same way `x-minienv` is keyed by
target: configure exactly one. The example above uses `ssh`; every transport's
fields are listed in
[Configuration Reference](./configuration-reference.md#docker).

### What teardown removes

`minienv destroy` runs `docker compose down --volumes --remove-orphans` and then
removes `~/.minienv/<namespace>` entirely. The directory holds a `.env` carrying
your ngrok auth token, so the removal happens even if the compose teardown
fails — the command still exits non-zero in that case, so you will see the
failure.
