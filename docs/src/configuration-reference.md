# Configuration Reference

Every field minienv reads. For explanations and examples, see
[Configuration](./configuration.md).

## `x-minienv`

Top level of the compose file. Configure exactly one of the below deployers —
configuring more than one is an error.

### `k8s`

| Field                     | Type     | Required | Default | Description                                                                       |
| ------------------------- | -------- | -------- | ------- | --------------------------------------------------------------------------------- |
| `k8s.context`             | string   | **yes**  | —       | Targets a specific cluster when deploying                                         |
| `k8s.namespace`           | string   | **yes**  | —       | Targets a specific namespace when deploying                                       |
| `k8s.deploymentTimeout`   | duration | no       | `60s`   | Helm timeout for all services; overridable per service                            |
| `k8s.ngrok.trafficPolicy` | string   | no       | `""`    | ngrok `on_http_request` policy applied to every service that does not set its own |

### `docker`

| Field                        | Type    | Required | Default    | Description                                                                       |
| ---------------------------- | ------- | -------- | ---------- | --------------------------------------------------------------------------------- |
| `docker.namespace`           | string  | **yes**  | —          | Compose project name on the remote host, and the directory it lives in            |
| `docker.ssh.host`            | string  | **yes**  | —          | Target host for the SSH connection                                                |
| `docker.ssh.identity`        | string  | **yes**  | —          | Path to the private key file                                                      |
| `docker.ssh.user`            | string  | no       | local user | User for the SSH connection                                                       |
| `docker.ssh.port`            | integer | no       | `22`       | Port for the SSH connection                                                       |
| `docker.ngrok.trafficPolicy` | string  | no       | `""`       | ngrok `on_http_request` policy applied to every service that does not set its own |

`docker.namespace` is reduced to `[a-z0-9_-]`, and rejected if nothing survives
that. The deployment lives at `~/.minienv/<namespace>` on the remote host, and
`minienv destroy` removes that directory.

Exactly one transport must be configured; configuring none, like configuring
more than one, is an error.

> **The host key must already be trusted.** minienv verifies against
> `~/.ssh/known_hosts` on the machine running it and offers no prompt or
> bypass, so connect once by hand before the first deploy. Encrypted identity
> files are not supported.

## `x-minienv-k8s-service`

Under `services.<name>`. All fields are optional.

| Field                        | Type                    | Default                      | Description                                                                                           |
| ---------------------------- | ----------------------- | ---------------------------- | ----------------------------------------------------------------------------------------------------- |
| `affinity`                   | k8s schema              | —                            | Passed through to the pod spec                                                                        |
| `command`                    | list of strings         | compose `command`            | Container command                                                                                     |
| `deploymentTimeout`          | duration                | inherits top level           | Helm timeout for this service                                                                         |
| `deploymentType`             | `service` \| `job`      | `service`                    | Deploy as a Deployment or a run-to-completion Job                                                     |
| `env`                        | map of string to string | —                            | Container env vars, merged over compose `environment`                                                 |
| `image.platforms`            | list of strings         | `["linux/amd64"]`            | Platforms to build and push                                                                           |
| `image.pullPolicy`           | string                  | `IfNotPresent`               | Kubernetes image pull policy                                                                          |
| `image.repository`           | string                  | from compose `image`         | Image repository                                                                                      |
| `image.tag`                  | string                  | from compose `image`         | Image tag. `+git` expands to the short commit SHA                                                     |
| `imagePullSecrets[].name`    | string                  | —                            | Name of an existing pull secret in the namespace                                                      |
| `livenessProbe`              | k8s schema              | from compose `healthcheck`   | Passed through to the container spec                                                                  |
| `manifests`                  | list of strings         | —                            | Paths, relative to compose-project-directory, to extra manifests rendered into this service's release |
| `ngrok.port`                 | integer                 | —                            | Container port to expose publicly. Use the **container** side of a compose mapping                    |
| `ngrok.trafficPolicy`        | string                  | inherits top level           | ngrok `on_http_request` policy                                                                        |
| `ngrok.url`                  | string                  | random                       | Reserved domain for a stable endpoint                                                                 |
| `nodeSelector`               | k8s schema              | —                            | Passed through to the pod spec                                                                        |
| `podAnnotations`             | map of string to string | —                            | Passed through to the pod template                                                                    |
| `podLabels`                  | map of string to string | —                            | Passed through to the pod template                                                                    |
| `podSecurityContext`         | k8s schema              | —                            | Passed through to the pod spec                                                                        |
| `readinessProbe`             | k8s schema              | from compose `healthcheck`   | Passed through to the container spec                                                                  |
| `recreate`                   | bool                    | `false`                      | Replaces the pods on every deploy, even unchanged ones                                                |
| `replicas`                   | integer                 | `1`                          | Number of replicas. No effect on a job                                                                |
| `resources`                  | k8s schema              | —                            | Passed through to the container spec                                                                  |
| `securityContext`            | k8s schema              | —                            | Passed through to the container spec                                                                  |
| `service.create`             | bool                    | `true`                       | Whether to create a Kubernetes Service. Forced to `false` when the service resolves no ports          |
| `service.ports[]`            | list                    | derived from compose `ports` | Explicit port mappings, merged with the derived ones                                                  |
| `service.type`               | string                  | `ClusterIP`                  | Service type                                                                                          |
| `serviceAccount.annotations` | map of string to string | `{}`                         | Extra annotations                                                                                     |
| `serviceAccount.automount`   | bool                    | `true`                       | Automount the service account token                                                                   |
| `serviceAccount.create`      | bool                    | `true`                       | Whether to create a ServiceAccount. Forced to `false` when the service resolves no ports              |
| `serviceAccount.name`        | string                  | `""`                         | Name. Empty means the generated fullname when creating, otherwise `default`                           |
| `skip`                       | bool                    | `false`                      | Excludes the service from image builds and deploys                                                    |
| `startupProbe`               | k8s schema              | from compose `healthcheck`   | Passed through to the container spec                                                                  |
| `tolerations`                | k8s schema              | —                            | Passed through to the pod spec                                                                        |
| `volumeMounts`               | k8s schema              | —                            | Passed through to the container spec                                                                  |
| `volumes`                    | k8s schema              | —                            | Passed through to the pod spec                                                                        |

### Notes

**Fields marked `k8s schema`** reach the generated manifests exactly as written,
so the [Kubernetes documentation](https://kubernetes.io/docs/reference/) is the
reference for their shape.

**The three probes** are derived from a compose `healthcheck`. A `curl` or
`wget` test against `http://localhost[:port][/path]` becomes an HTTP probe on
the **container** port; anything else becomes an exec probe. `disable: true`, an
empty `test`, or `test: ["NONE"]` produces none.

Set any of the three to write it yourself. The ones you leave unset still come
from the compose `healthcheck`:

```yaml
x-minienv-k8s-service:
  readinessProbe:
    httpGet:
      path: /ready
      port: 8080
    initialDelaySeconds: 5
```

> **Caution:** a `curl` or `wget` test pointed at anything other than
> `localhost` produces **no probes at all** — silently. Use `localhost`, write
> the check as a non-HTTP command, or set the probes yourself.

**`env`** is merged over compose `environment` and reaches the container as
plain env vars — no ConfigMap, no Secret — so **do not put secrets there** if
the manifests are visible to others. Values compose could not resolve (the bare
`- SOME_VAR` form, with nothing set locally) are dropped rather than set empty.

**`skip`** leaves a service out of image builds and out of the deploy, for the
ones that only make sense locally. `destroy` does not check it, so a service you
deployed and later marked skipped is still uninstalled.

**Each entry in `service.ports[]`** requires all three fields:

| Field               | Type    | Description                                    |
| ------------------- | ------- | ---------------------------------------------- |
| `containerPortName` | string  | Name of the port, on the container and Service |
| `containerPort`     | integer | Port the container listens on                  |
| `protocol`          | string  | `TCP` or `UDP`                                 |

Ports derived from compose take the container side of the mapping, are named
`p<port>` — `p8080` for container port 8080 — and default to protocol `TCP`.

**`manifests`** names files, relative to the compose project directory, that are
deployed as part of the service's release — a ConfigMap or Secret behind a
`volumes` entry, an Ingress, a PVC, a Traefik `IngressRoute`:

```yaml
services:
  api:
    image: myorg/api:latest
    x-minienv-k8s-service:
      manifests:
        - k8s/configmap.yaml
      volumes:
        - name: config
          configMap:
            name: api-config
      volumeMounts:
        - name: config
          mountPath: /etc/api
```

Each file is a Helm template. `.Values`, `.Release` and `.Chart` are in scope,
alongside the chart's `generated.name`, `generated.fullname`, `generated.chart`,
`generated.labels`, `generated.selectorLabels` and
`generated.serviceAccountName` helpers:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ include "generated.fullname" . }}-config
  labels:
    {{- include "generated.labels" . | nindent 4 }}
data:
  LOG_LEVEL: debug
```

Every field in the table above is readable as `.Values` under the same path —
`replicas` as `.Values.replicas`, `image.tag` as `.Values.image.tag` — resolved,
so `.Values.image.tag` carries the tag derived from compose `image:` with a
`+git` already expanded.

> **A job's values are a smaller set.** A `deploymentType: job` chart has no
> `replicas` or probes, so a manifest on a job that reads one renders empty
> rather than failing.

A manifest is created and removed with its service's release. Two services must
not declare manifests producing the same object — the second release to reach it
fails on ownership, and the order between them is not fixed.

**`ngrok.*`** requires `NGROK_AUTHTOKEN`; without it the whole block is ignored.
Ignored for `deploymentType: job` and for a service marked `skip`, since neither
has a k8s Service for an endpoint to route to. See
[Exposing Services](./exposing-services.md).

**`recreate`** exists because Helm only restarts pods when something in the
release actually changed. A service whose image tag is fixed — `latest`, or any
tag you rebuild in place — keeps running the old image after a redeploy, because
from Helm's point of view nothing about the release moved.

```yaml
services:
  api:
    image: myorg/api:latest
    x-minienv-k8s-service:
      recreate: true
```

The alternative is to make the tag change instead, with `+git` — see
[Images and Builds](./images-and-builds.md). It applies to the whole release,
not just the pods: any resource that cannot be patched in place is replaced,
including one named in `manifests`.

## `x-minienv-docker-service`

Under `services.<name>`. All fields are optional.

The field set is small because compose is already the deployment format:
replicas, commands, environment, resources, health checks and volumes stay on
the compose service and are passed through untouched.

| Field                 | Type            | Default              | Description                                                                        |
| --------------------- | --------------- | -------------------- | ---------------------------------------------------------------------------------- |
| `image.platforms`     | list of strings | `["linux/amd64"]`    | Platforms to build and push                                                        |
| `image.repository`    | string          | from compose `image` | Image repository                                                                   |
| `image.tag`           | string          | from compose `image` | Image tag. `+git` expands to the short commit SHA                                  |
| `ngrok.port`          | integer         | —                    | Container port to expose publicly. Use the **container** side of a compose mapping |
| `ngrok.trafficPolicy` | string          | inherits top level   | ngrok `on_http_request` policy                                                     |
| `ngrok.url`           | string          | random               | Reserved domain for a stable endpoint                                              |
| `skip`                | bool            | `false`              | Excludes the service from image builds and deploys                                 |

### Notes

**`ngrok.*`** requires `NGROK_AUTHTOKEN`; without it the whole block is ignored.
Ignored for a service marked `skip`, since it is never deployed. See
[Exposing Services](./exposing-services.md).

**`skip`** leaves a service out of image builds and out of the deploy, for the
ones that only make sense locally.

**Compose `environment`** reaches the remote host already interpolated, so
`${DB_HOST}` arrives as the value it had on your machine. To keep a value off
the remote entirely, use the bare `- SOME_VAR` form: with nothing set locally it
stays unresolved, and the remote host supplies it from its own environment.

### What the rewrite changes

Before the project reaches the remote host, minienv:

| Change                             | Why                                                       |
| ---------------------------------- | --------------------------------------------------------- |
| Renames the project to `namespace` | Keeps two environments on one host from colliding         |
| Drops services marked `skip`       | Along with any `depends_on` edge pointing at them         |
| Clears `build:`                    | Images are built locally and pulled by tag on the remote  |
| Clears `ports:`                    | Nothing is published to the remote host's network         |
| Drops bind mounts                  | The host paths do not exist there. Named volumes are kept |
| Drops `env_file` and `label_file`  | Their values are already in `environment` and `labels`    |
| Pins `image:`                      | To the repository and tag resolved for this run           |
| Injects an `ngrok` service         | Only when something publishes                             |

Everything else — `environment`, `healthcheck`, `command`, `depends_on`,
`networks`, named `volumes` — is passed through as written, already fully
interpolated.

## Generated resources

### Kubernetes

Per compose service, minienv creates:

| Resource       | When                                                   |
| -------------- | ------------------------------------------------------ |
| Deployment     | `deploymentType: service` (the default)                |
| Job            | `deploymentType: job`                                  |
| Service        | `deploymentType: service` and `service.create` is true |
| ServiceAccount | `serviceAccount.create` is true                        |

And once per namespace, not per service:

| Resource                                                       | When                            |
| -------------------------------------------------------------- | ------------------------------- |
| `ngrok` release: Deployment, ConfigMap, Secret, ServiceAccount | any service publishes via ngrok |

The target namespace is created on deploy if it does not already exist.

Nothing else is derived from the compose file — no Ingress,
PersistentVolumeClaim, HorizontalPodAutoscaler or PodDisruptionBudget, and
`environment` is rendered inline rather than into a ConfigMap or Secret.
Anything else a service needs is declared explicitly, with `manifests`.

## Machine-readable schema

The authoritative schema lives at `schema/schema.json` in the repository. It
wraps the official compose spec and adds the three extension fields
(`x-minienv`, `x-minienv-k8s-service` and `x-minienv-docker-service`). Reference
it from your compose file for editor autocomplete:

```yaml
# $schema: https://raw.githubusercontent.com/robgonnella/minienv/refs/heads/main/schema/schema.json
```
