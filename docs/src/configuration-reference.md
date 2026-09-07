# Configuration Reference

Every field, in tables. For explanations and examples, see
[Configuration](./configuration.md).

## `x-minienv`

Top level of the compose file.

| Field                   | Type     | Required | Default | Description                                                                       |
| ----------------------- | -------- | -------- | ------- | --------------------------------------------------------------------------------- |
| `k8s.context`           | string   | **yes**  | —       | Targets a specific cluster when deploying                                         |
| `k8s.namespace`         | string   | **yes**  | —       | Targets a specific namespace when deploying                                       |
| `k8s.deploymentTimeout` | duration | no       | `60s`   | Helm timeout for all services; overridable per service                            |
| `ngrok.trafficPolicy`   | string   | no       | `""`    | ngrok `on_http_request` policy applied to every service that does not set its own |

## `x-minienv-k8s-service`

Under `services.<name>`. All fields are optional.

### Deployment

| Field               | Type                    | Default            | Description                                            |
| ------------------- | ----------------------- | ------------------ | ------------------------------------------------------ |
| `skip`              | bool                    | `false`            | Excludes the service from image builds and deploys     |
| `deploymentType`    | `service` \| `job`      | `service`          | Deploy as a Deployment or a run-to-completion Job      |
| `deploymentTimeout` | duration                | inherits top level | Helm timeout for this service                          |
| `recreate`          | bool                    | `false`            | Replaces the pods on every deploy, even unchanged ones |
| `replicas`          | integer                 | `1`                | Number of replicas. No effect on a job                 |
| `command`           | list of strings         | compose `command`  | Container command                                      |
| `env`               | map of string to string | —                  | Container env vars, merged over compose `environment`  |

#### recreate

Helm only restarts pods when something in the release actually changed. A
service whose image tag is fixed — `latest`, or any tag you rebuild in place —
therefore keeps running the old image after a redeploy, because from Helm's
point of view nothing about the release moved.

`recreate: true` forces the pods to be replaced on every deploy:

```yaml
services:
  api:
    image: myorg/api:latest
    x-minienv-k8s-service:
      recreate: true
```

The alternative is to make the tag change instead, with `+git` — see
[Images and Builds](./images-and-builds.md).

### Image

| Field                     | Type            | Default              | Description                                       |
| ------------------------- | --------------- | -------------------- | ------------------------------------------------- |
| `image.repository`        | string          | from compose `image` | Image repository                                  |
| `image.tag`               | string          | from compose `image` | Image tag. `+git` expands to the short commit SHA |
| `image.pullPolicy`        | string          | `IfNotPresent`       | Kubernetes image pull policy                      |
| `image.platforms`         | list of strings | `["linux/amd64"]`    | Platforms to build and push                       |
| `imagePullSecrets[].name` | string          | —                    | Name of an existing pull secret in the namespace  |

`repository` and `tag` are required in the sense that the deploy fails without
them — but minienv derives both from a compose `image: repo:tag` when the
extension omits them.

### Service and ports

| Field             | Type   | Default                      | Description                                                                                  |
| ----------------- | ------ | ---------------------------- | -------------------------------------------------------------------------------------------- |
| `service.create`  | bool   | `true`                       | Whether to create a Kubernetes Service. Forced to `false` when the service resolves no ports |
| `service.type`    | string | `ClusterIP`                  | Service type                                                                                 |
| `service.ports[]` | list   | derived from compose `ports` | Explicit port mappings, merged with the derived ones                                         |

Each entry in `service.ports` requires all five fields:

| Field               | Type    | Description                   |
| ------------------- | ------- | ----------------------------- |
| `containerPortName` | string  | Name of the container port    |
| `containerPort`     | integer | Port the container listens on |
| `servicePortName`   | string  | Name of the service port      |
| `servicePort`       | integer | Port the Service exposes      |
| `protocol`          | string  | `TCP` or `UDP`                |

Ports derived from compose are named `p<port>` — `p8080` for container port
8080 — and default to protocol `TCP`.

### ServiceAccount

| Field                        | Type                    | Default | Description                                                                              |
| ---------------------------- | ----------------------- | ------- | ---------------------------------------------------------------------------------------- |
| `serviceAccount.create`      | bool                    | `true`  | Whether to create a ServiceAccount. Forced to `false` when the service resolves no ports |
| `serviceAccount.name`        | string                  | `""`    | Name. Empty means the generated fullname when creating, otherwise `default`              |
| `serviceAccount.automount`   | bool                    | `true`  | Automount the service account token                                                      |
| `serviceAccount.annotations` | map of string to string | `{}`    | Extra annotations                                                                        |

### ngrok

| Field                 | Type    | Default            | Description                                                                                                                                 |
| --------------------- | ------- | ------------------ | ------------------------------------------------------------------------------------------------------------------------------------------- |
| `ngrok.port`          | integer | —                  | Service port to expose publicly. Must match a resolved service port; use the **host** side of a compose mapping                             |
| `ngrok.url`           | string  | random             | ngrok URL for a stable endpoint. The domain must already be reserved on the account. Interpolate `${MINIENV_NAMESPACE}` on a shared account |
| `ngrok.trafficPolicy` | string  | inherits top level | ngrok `on_http_request` policy                                                                                                              |

Requires `NGROK_AUTHTOKEN` — without it the whole block is ignored.
`NGROK_API_KEY` is optional and only used to print the URL table.
Ignored for `deploymentType: job` and for a service marked `skip`, since neither
has a k8s Service for an endpoint to route to. See
[Exposing Services](./exposing-services.md).

### Kubernetes passthrough fields

These are passed through to the generated manifests exactly as written, so the
[Kubernetes documentation](https://kubernetes.io/docs/reference/) is the
reference for their shape:

`podAnnotations`, `podLabels`, `podSecurityContext`, `securityContext`,
`resources`, `startupProbe`, `livenessProbe`, `readinessProbe`, `volumes`,
`volumeMounts`, `nodeSelector`, `tolerations`, `affinity`

The three probes default to whatever a compose `healthcheck` produces; setting
one explicitly replaces that.

## Generated resources

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

No Ingress, PersistentVolumeClaim, HorizontalPodAutoscaler, or
PodDisruptionBudget is ever generated, and compose `environment` values are
rendered inline rather than into a ConfigMap or Secret.

## Machine-readable schema

The authoritative schema lives at `schema/schema.json` in the repository. It
wraps the official compose spec and adds the two extension fields. Reference it
from your compose file for editor autocomplete:

```yaml
# $schema: https://raw.githubusercontent.com/robgonnella/minienv/refs/heads/main/schema/schema.json
```
