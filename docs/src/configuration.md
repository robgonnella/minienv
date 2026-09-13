# Configuration

minienv adds one top-level field to your compose file, plus a per-service field
for whichever target you picked.

| Field                      | Where           | Purpose                                      |
| -------------------------- | --------------- | -------------------------------------------- |
| `x-minienv`                | top level       | Chooses and configures the deployment target |
| `x-minienv-k8s-service`    | under a service | Per-service overrides, Kubernetes target     |
| `x-minienv-docker-service` | under a service | Per-service overrides, Docker target         |

All three are standard compose extension fields, so `docker compose` ignores
them and the same file keeps working locally.

Use the service extension matching your target. The other one is not an error —
it is simply ignored, which is worth remembering when a setting seems to have no
effect.

## Choosing a target

Configure exactly one. Configuring both is an error; configuring neither means
minienv has no target and refuses to run.

```yaml
x-minienv:
  k8s:
    context: minikube # required
    namespace: my-branch # required
    deploymentTimeout: 60s # optional, default 60s
    ngrok:
      trafficPolicy: "" # optional, applies to all services
```

```yaml
x-minienv:
  docker:
    namespace: my-branch # required
    ssh: # exactly one transport is required
      host: dev-box.example.com # required
      identity: ~/.ssh/id_ed25519 # required
      user: me # optional, defaults to the local user
      port: 22 # optional, default 22
    ngrok:
      trafficPolicy: "" # optional, applies to all services
```

## Per-service overrides

Entirely optional — most services need nothing here. Every field is listed in
the [Configuration Reference](./configuration-reference.md).

### Kubernetes

```yaml
services:
  api:
    image: myorg/api:v1
    ports:
      - "8080:8080"
    x-minienv-k8s-service:
      replicas: 2
      resources:
        limits:
          memory: 512Mi
```

Anything set here overrides the equivalent compose value.

### Docker

```yaml
services:
  api:
    image: myorg/api:v1
    ports:
      - "8080:8080"
    x-minienv-docker-service:
      image:
        tag: +git
      ngrok:
        port: 8080
```

The field set is small because compose is already the deployment format:
commands, environment, health checks and volumes stay on the compose service.

## What to change in your compose file

Most files deploy as written. These are the cases that need an edit.

### Kubernetes

- **Write ports as `host:container`.** The bare `- "8080"` form is valid compose
  but an error here: the container side becomes the pod's `containerPort` and
  the host side the Service port, so both are needed.
- **Move an `entrypoint` override into `command`.** `entrypoint` is not read.
- **Declare storage in the extension** rather than in compose `volumes`:

  ```yaml
  x-minienv-k8s-service:
    volumes:
      - name: cache
        emptyDir: {}
    volumeMounts:
      - name: cache
        mountPath: /var/cache
  ```

Also not read: `networks`, `labels`, `deploy`, `restart`, `profiles`, `user`,
`working_dir`, `extra_hosts`, `expose`, and top-level `volumes`, `networks`,
`secrets` and `configs`. A service using one still deploys — it just behaves
differently than it does locally.

### Docker

- **Replace bind mounts with named volumes.** Host paths do not exist on the
  remote, so bind mounts are dropped and named volumes are kept.
- **Avoid `secrets` and `configs` declared with `file:`.** The local path is
  carried over as written and will not resolve on the remote host.

Everything else is passed through as written.

## Health checks

### Kubernetes

A compose `healthcheck` becomes all three probes — startup, liveness and
readiness.

```yaml
healthcheck:
  test: curl http://localhost:8080/healthz
  interval: 10s
  timeout: 2s
  retries: 3
```

A `curl` or `wget` test against `http://localhost[:port][/path]` becomes an HTTP
probe, using the **container** port. Anything else becomes an exec probe.
`disable: true`, an empty `test`, or `test: ["NONE"]` produces no probes.

> **Caution:** a `curl` or `wget` test pointed at anything other than
> `localhost` produces **no probes at all** — silently. Use `localhost`, or
> write the check as a non-HTTP command.

### Docker

A compose `healthcheck` runs on the remote host exactly as it runs locally,
along with the `depends_on` conditions that build on it. There is nothing to
configure.

## Environment variables

### Kubernetes

Compose `environment` is copied to the container as plain env vars — no
ConfigMap, no Secret — so **do not put secrets there** if the manifests are
visible to others. Values compose could not resolve (the bare `- SOME_VAR` form,
with nothing set locally) are dropped rather than set empty.

To add or override at deploy time only:

```yaml
x-minienv-k8s-service:
  env:
    LOG_LEVEL: debug
```

### Docker

Compose `environment` reaches the remote host already interpolated, so
`${DB_HOST}` arrives as the value it had on your machine.

To keep a value off the remote entirely, use the bare `- SOME_VAR` form: with
nothing set locally it stays unresolved, and the remote host supplies it from
its own environment.

## Skipping a service

Some services only make sense locally — a mail catcher, a mock, a debug proxy.
`skip: true` leaves the service out of image builds and out of the deploy.

```yaml
services:
  mailhog:
    image: mailhog/mailhog
    x-minienv-k8s-service:
      skip: true
```

Use `x-minienv-docker-service` for the docker target.

> **Kubernetes:** `destroy` does not check `skip`. If you deploy a service and
> later mark it skipped, `minienv destroy` still uninstalls it — which is
> generally what you want, since it cleans up what an earlier deploy created.

## Next steps

- [Images and Builds](./images-and-builds.md)
- [Exposing Services](./exposing-services.md)
- [Jobs and Dependencies](./jobs-and-dependencies.md)
