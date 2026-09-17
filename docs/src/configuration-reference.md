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

| Field                                  | Type    | Required | Default    | Description                                                                                               |
| -------------------------------------- | ------- | -------- | ---------- | --------------------------------------------------------------------------------------------------------- |
| `docker.namespace`                     | string  | **yes**  | —          | Compose project name and directory (`~/.minienv/<namespace>`) on the remote host. Reduced to `[a-z0-9_-]` |
| [`docker.transport`](#dockertransport) | map     | **yes**  | —          | How to reach the remote host                                                                              |
| `docker.transport.ssh.host`            | string  | **yes**  | —          | Target host for the SSH connection                                                                        |
| `docker.transport.ssh.identity`        | string  | **yes**  | —          | Path to the private key file                                                                              |
| `docker.transport.ssh.user`            | string  | no       | local user | User for the SSH connection                                                                               |
| `docker.transport.ssh.port`            | integer | no       | `22`       | Port for the SSH connection                                                                               |
| `docker.ngrok.trafficPolicy`           | string  | no       | `""`       | ngrok `on_http_request` policy applied to every service that does not set its own                         |

#### `docker.transport`

Exactly one transport must be configured; configuring none, like configuring
more than one, is an error.

> **The host key must already be trusted.** minienv verifies against
> `~/.ssh/known_hosts` on the machine running it and offers no prompt or
> bypass, so connect once by hand before the first deploy. Encrypted identity
> files are not supported.

## `x-minienv-k8s-service`

Under `services.<name>`. All fields are optional.

| Field                             | Type                    | Default                      | Description                                                                                                                                                                                                 |
| --------------------------------- | ----------------------- | ---------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `affinity`                        | k8s schema              | —                            | Passed through to the pod spec                                                                                                                                                                              |
| `command`                         | list of strings         | compose `command`            | Container command                                                                                                                                                                                           |
| [`configMapFrom`](#configmapfrom) | list of strings         | —                            | Files, relative to the compose project directory, that become one ConfigMap named after the service                                                                                                         |
| `deploymentTimeout`               | duration                | inherits top level           | Helm timeout for this service                                                                                                                                                                               |
| `deploymentType`                  | `service` \| `job`      | `service`                    | Deploy as a Deployment or a run-to-completion Job                                                                                                                                                           |
| [`env`](#env)                     | k8s schema              | —                            | Container env vars as `EnvVar` entries, merged over compose `environment` by `name`                                                                                                                         |
| `envFrom`                         | k8s schema              | —                            | Sources to populate container env vars from, as `EnvFromSource` entries                                                                                                                                     |
| `image.platforms`                 | list of strings         | `["linux/amd64"]`            | Platforms to build and push                                                                                                                                                                                 |
| `image.pullPolicy`                | string                  | `IfNotPresent`               | Kubernetes image pull policy                                                                                                                                                                                |
| `image.repository`                | string                  | from compose `image`         | Image repository                                                                                                                                                                                            |
| `image.tag`                       | string                  | from compose `image`         | Image tag. `+git` expands to the short commit SHA                                                                                                                                                           |
| `imagePullSecrets[].name`         | string                  | —                            | Existing pull secret in the namespace. See [Private registries](./images-and-builds.md#private-registries)                                                                                                  |
| [`livenessProbe`](#probes)        | k8s schema              | from compose `healthcheck`   | Passed through to the container spec                                                                                                                                                                        |
| [`manifests`](#manifests)         | list of strings         | —                            | Paths, relative to the compose project directory, to extra manifests rendered into this service's release                                                                                                   |
| `ngrok.port`                      | integer                 | —                            | Container port to expose publicly — the **container** side of a compose mapping. Ignored for a job, a skipped service, and when `NGROK_AUTHTOKEN` is unset. See [Exposing Services](./exposing-services.md) |
| `ngrok.trafficPolicy`             | string                  | inherits top level           | ngrok `on_http_request` policy                                                                                                                                                                              |
| `ngrok.url`                       | string                  | random                       | Reserved domain for a stable endpoint                                                                                                                                                                       |
| `nodeSelector`                    | k8s schema              | —                            | Passed through to the pod spec                                                                                                                                                                              |
| `podAnnotations`                  | map of string to string | —                            | Passed through to the pod template                                                                                                                                                                          |
| `podLabels`                       | map of string to string | —                            | Passed through to the pod template                                                                                                                                                                          |
| `podSecurityContext`              | k8s schema              | —                            | Passed through to the pod spec                                                                                                                                                                              |
| [`readinessProbe`](#probes)       | k8s schema              | from compose `healthcheck`   | Passed through to the container spec                                                                                                                                                                        |
| `recreate`                        | bool                    | `false`                      | Replace the pods, and any resource that cannot be patched in place, on every deploy. For fixed tags like `latest`                                                                                           |
| `replicas`                        | integer                 | `1`                          | Number of replicas. No effect on a job                                                                                                                                                                      |
| `resources`                       | k8s schema              | —                            | Passed through to the container spec                                                                                                                                                                        |
| `securityContext`                 | k8s schema              | —                            | Passed through to the container spec                                                                                                                                                                        |
| `service.create`                  | bool                    | `true`                       | Whether to create a Kubernetes Service. Forced to `false` when the service resolves no ports                                                                                                                |
| [`service.ports`](#serviceports)  | list                    | derived from compose `ports` | Explicit port mappings, merged with the derived ones                                                                                                                                                        |
| `service.type`                    | string                  | `ClusterIP`                  | Service type                                                                                                                                                                                                |
| `serviceAccount.annotations`      | map of string to string | `{}`                         | Extra annotations                                                                                                                                                                                           |
| `serviceAccount.automount`        | bool                    | `true`                       | Automount the service account token                                                                                                                                                                         |
| `serviceAccount.create`           | bool                    | `true`                       | Whether to create a ServiceAccount. Forced to `false` when the service resolves no ports                                                                                                                    |
| `serviceAccount.name`             | string                  | `""`                         | Name. Empty means the generated fullname when creating, otherwise `default`                                                                                                                                 |
| `skip`                            | bool                    | `false`                      | Excludes the service from image builds and deploys. `destroy` still uninstalls it                                                                                                                           |
| [`startupProbe`](#probes)         | k8s schema              | from compose `healthcheck`   | Passed through to the container spec                                                                                                                                                                        |
| `tolerations`                     | k8s schema              | —                            | Passed through to the pod spec                                                                                                                                                                              |
| `volumeMounts`                    | k8s schema              | —                            | Passed through to the container spec                                                                                                                                                                        |
| `volumes`                         | k8s schema              | —                            | Passed through to the pod spec                                                                                                                                                                              |

Fields marked `k8s schema` reach the generated manifests exactly as written, so
the [Kubernetes documentation](https://kubernetes.io/docs/reference/) is the
reference for their shape.

#### `configMapFrom`

Each file's base name is a key and its content is the value. Mount the
ConfigMap with `volumes` and `volumeMounts`:

```yaml
services:
  postgres:
    image: postgres:15
    x-minienv-k8s-service:
      configMapFrom:
        - db/init/01-schema.sql
        - db/init/02-seed.sql
      volumes:
        - name: init
          configMap:
            name: postgres
      volumeMounts:
        - name: init
          mountPath: /docker-entrypoint-initdb.d
```

Content is written as-is, not rendered as a template, and must be UTF-8 text.
Two entries may not share a base name. A change to any file's content replaces
the pods on the next deploy. A file under `manifests` must not also produce a
ConfigMap named after the service, since the two would collide in the release.

#### `env`

Merged over compose `environment` by `name`. A literal `value` is written into
the manifest, so **do not put secrets there** if the manifests are visible to
others. Values compose could not resolve (the bare `- SOME_VAR` form, with
nothing set locally) are dropped rather than set empty.

```yaml
x-minienv-k8s-service:
  env:
    - name: EXAMPLE_VAR
      value: example-value
    - name: EXAMPLE_FROM_VAR
      valueFrom:
        secretKeyRef:
          name: example-secret
          key: example-key
```

#### `manifests`

Files deployed as part of the service's release — a ConfigMap or Secret behind
a `volumes` entry, an Ingress, a PVC, a Traefik `IngressRoute`:

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

#### Probes

`livenessProbe`, `readinessProbe` and `startupProbe` are derived from a compose
`healthcheck`. A `curl` or `wget` test against `http://localhost[:port][/path]`
becomes an HTTP probe on the **container** port; anything else becomes an exec
probe. `disable: true`, an empty `test`, or `test: ["NONE"]` produces none.

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

#### `service.ports`

Each entry requires all three fields:

| Field               | Type    | Description                                    |
| ------------------- | ------- | ---------------------------------------------- |
| `containerPortName` | string  | Name of the port, on the container and Service |
| `containerPort`     | integer | Port the container listens on                  |
| `protocol`          | string  | `TCP` or `UDP`                                 |

Ports derived from compose take the container side of the mapping, are named
`p<port>` — `p8080` for container port 8080 — and default to protocol `TCP`.

## `x-minienv-docker-service`

Under `services.<name>`. All fields are optional.

| Field                           | Type            | Default              | Description                                                                                                                                                                                         |
| ------------------------------- | --------------- | -------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [`copy[].containerPath`](#copy) | string          | —                    | Absolute path inside the container to mount the copied path at                                                                                                                                      |
| [`copy[].hostPath`](#copy)      | string          | —                    | Path, relative to the compose project directory, of a file or directory to copy                                                                                                                     |
| `image.platforms`               | list of strings | `["linux/amd64"]`    | Platforms to build and push                                                                                                                                                                         |
| `image.repository`              | string          | from compose `image` | Image repository                                                                                                                                                                                    |
| `image.tag`                     | string          | from compose `image` | Image tag. `+git` expands to the short commit SHA                                                                                                                                                   |
| `ngrok.port`                    | integer         | —                    | Container port to expose publicly — the **container** side of a compose mapping. Ignored for a skipped service and when `NGROK_AUTHTOKEN` is unset. See [Exposing Services](./exposing-services.md) |
| `ngrok.trafficPolicy`           | string          | inherits top level   | ngrok `on_http_request` policy                                                                                                                                                                      |
| `ngrok.url`                     | string          | random               | Reserved domain for a stable endpoint                                                                                                                                                               |
| `skip`                          | bool            | `false`              | Excludes the service from image builds and deploys                                                                                                                                                  |

#### `copy`

Files and directories sent to the remote host and bind mounted into the
container — a config file, a TLS certificate, a seed dataset, a directory of
fixtures:

```yaml
services:
  api:
    image: myorg/api:latest
    x-minienv-docker-service:
      copy:
        - hostPath: conf/api.yml
          containerPath: /etc/api/api.yml
        - hostPath: seed
          containerPath: /var/lib/seed
```

A directory is copied recursively. `hostPath` must stay inside the project — an
absolute path, a path climbing out with `..`, and the project directory itself
are all rejected. `containerPath` must be absolute, and two entries on one
service cannot name the same one.

The copied paths land under the deployment's own directory on the remote host,
each keeping the relative path it was declared with, so two files that share a
base name stay apart. Two services naming the same `hostPath` share one copy. A
service marked `skip` is not deployed, so nothing it declares is copied.

> **The whole set is replaced on every deploy.** An entry you remove or rename
> takes its remote copy with it on the next deploy, and `minienv destroy`
> removes the deployment directory that holds all of them.

## Machine-readable schema

The authoritative schema lives at `schema/schema.json` in the repository. It
wraps the official compose spec and adds the three extension fields
(`x-minienv`, `x-minienv-k8s-service` and `x-minienv-docker-service`). Reference
it from your compose file for editor autocomplete:

```yaml
# $schema: https://raw.githubusercontent.com/robgonnella/minienv/refs/heads/main/schema/schema.json
```
