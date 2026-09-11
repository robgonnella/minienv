# Configuration

minienv adds one top-level field to your compose file, plus a per-service field
for whichever deployment target you picked.

| Field                      | Where           | Purpose                                      |
| -------------------------- | --------------- | -------------------------------------------- |
| `x-minienv`                | top level       | Chooses and configures the deployment target |
| `x-minienv-k8s-service`    | under a service | Per-service overrides, Kubernetes target     |
| `x-minienv-docker-service` | under a service | Per-service overrides, Docker target         |

All are standard compose extension fields, so `docker compose` ignores them.
The same file keeps working locally.

Use the service extension matching your active deployer. The other one is not an
error — it is simply ignored, which is worth remembering when a setting seems to
have no effect.

## `x-minienv`

Configure exactly one target. Configuring both is an error; configuring neither
means minienv has no target and refuses to run.

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

For `k8s`, `context` and `namespace` are both required. For `docker`,
`namespace` is required along with exactly one transport block.

## `x-minienv-k8s-service`

Entirely optional. Most services need nothing here; minienv derives what it can
from standard compose fields.

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

**Precedence:** anything set in `x-minienv-k8s-service` overrides the equivalent
compose value.

## What minienv reads from compose

| Compose field                                     | Becomes                                                                                                   |
| ------------------------------------------------- | --------------------------------------------------------------------------------------------------------- |
| `image`                                           | The container image. Split on the first `:` into repository and tag when the extension does not set them. |
| `build.context`, `build.dockerfile`, `build.args` | Built and pushed with `docker buildx bake` before deploying.                                              |
| `ports`                                           | Container ports and a Kubernetes Service.                                                                 |
| `environment`                                     | Container env vars, rendered inline.                                                                      |
| `command`                                         | The container `command`.                                                                                  |
| `healthcheck`                                     | Startup, liveness, and readiness probes.                                                                  |
| `depends_on`                                      | Deploy order (and reverse order on destroy).                                                              |

Everything else is ignored — see below.

## What minienv ignores

These compose fields have **no effect**. They are not errors; they are simply
not read, so a service can deploy successfully and still not behave the way your
local `docker compose up` does.

- Top level: `volumes`, `networks`, `secrets`, `configs`
- Per service: `volumes`, `networks`, `entrypoint`, `labels`, `deploy`,
  `restart`, `profiles`, `user`, `working_dir`, `extra_hosts`, `expose`

Two of these bite most often:

- **`entrypoint` is ignored while `command` is honored.** If your image relies
  on a compose `entrypoint` override, move it to `command`.
- **`volumes` is ignored.** Container storage goes through the extension's
  `volumes` and `volumeMounts` keys instead, which take Kubernetes syntax:

  ```yaml
  x-minienv-k8s-service:
    volumes:
      - name: cache
        emptyDir: {}
    volumeMounts:
      - name: cache
        mountPath: /var/cache
  ```

## Ports

Port mappings must be written as `host:container`:

```yaml
ports:
  - "8080:8080" # good
  - "8080" # error
```

The bare form is valid compose but an error in minienv, because it needs both
sides: the container side becomes the pod's `containerPort`, and the host side
becomes the Service port.

A service with no ports gets no Service and no ServiceAccount — just a
Deployment. That is usually what you want for a worker.

## Environment variables

Compose `environment` is copied to the container as plain env vars. minienv does
not create a ConfigMap or Secret for them, so **do not put secrets in
`environment`** if the manifest is visible to others.

Values that compose could not resolve (the bare `- SOME_VAR` pass-through form,
with nothing set on the host) are dropped rather than set to an empty string.

To add or override variables at deploy time only:

```yaml
x-minienv-k8s-service:
  env:
    LOG_LEVEL: debug
```

## Health checks

A compose `healthcheck` is converted into all three Kubernetes probes — startup,
liveness, and readiness.

```yaml
healthcheck:
  test: curl http://localhost:8080/healthz
  interval: 10s
  timeout: 2s
  retries: 3
```

The rules:

- A `curl` or `wget` test against `http://localhost[:port][/path]` becomes an
  **HTTP probe**. The port in that URL must be the **container** port, not the
  host port.
- Any other test becomes an **exec probe** run through `/bin/sh -c`.
- `interval`, `timeout`, and `retries` map to `periodSeconds`,
  `timeoutSeconds`, and `failureThreshold`.
- `disable: true`, an empty `test`, or `test: ["NONE"]` produces no probes.

> **Caution:** a `curl` or `wget` test pointed at anything other than
> `localhost` produces **no probes at all** — silently. Use `localhost` or write
> the check as a non-HTTP command.

Setting `startupProbe`, `livenessProbe`, or `readinessProbe` in the extension
overrides whatever the healthcheck would have produced for that probe.

## Skipping a service

Some services only make sense locally — a mail catcher, a mock, a debug proxy.

```yaml
services:
  mailhog:
    image: mailhog/mailhog
    x-minienv-k8s-service:
      skip: true
```

`skip: true` leaves the service out of both image builds and deploys.

> **Note:** `destroy` does not check `skip`. If you deploy a service and later
> mark it skipped, `minienv destroy` will still uninstall it — which is
> generally what you want, since it cleans up what an earlier deploy created.

## Next steps

- [Images and Builds](./images-and-builds.md)
- [Exposing Services](./exposing-services.md)
- [Jobs and Dependencies](./jobs-and-dependencies.md)
