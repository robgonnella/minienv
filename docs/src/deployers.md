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

### Generated resources

Per compose service:

| Resource       | When                                                   |
| -------------- | ------------------------------------------------------ |
| Deployment     | `deploymentType: service` (the default)                |
| Job            | `deploymentType: job`                                  |
| Service        | `deploymentType: service` and `service.create` is true |
| ServiceAccount | `serviceAccount.create` is true                        |
| ConfigMap      | `configMapFrom` names at least one file                |

Once per namespace:

| Resource                                                       | When                            |
| -------------------------------------------------------------- | ------------------------------- |
| `ngrok` release: Deployment, ConfigMap, Secret, ServiceAccount | any service publishes via ngrok |

The target namespace is created on deploy if it does not already exist. On
destroy it is left in place, unless `removeNamespaceOnDestroy` is `true` — then
the whole namespace is deleted, including anything in it that minienv did not
create.

Nothing else is derived from the compose file — no Ingress,
PersistentVolumeClaim, HorizontalPodAutoscaler or PodDisruptionBudget, and
compose `environment` is rendered inline rather than into a ConfigMap or
Secret. Anything else a service needs is declared explicitly, with
`configMapFrom` or `manifests`.

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

### What reaches the remote host

Before the project is sent, minienv:

| Change                             | Why                                                       |
| ---------------------------------- | --------------------------------------------------------- |
| Renames the project to `namespace` | Keeps two environments on one host from colliding         |
| Drops services marked `skip`       | Along with any `depends_on` edge pointing at them         |
| Clears `build:`                    | Images are built locally and pulled by tag on the remote  |
| Clears `ports:`                    | Nothing is published to the remote host's network         |
| Drops bind mounts                  | The host paths do not exist there. Named volumes are kept |
| Adds a bind mount per `copy` entry | Pointed at where that entry is copied on the host         |
| Drops `env_file` and `label_file`  | Their values are already in `environment` and `labels`    |
| Pins `image:`                      | To the repository and tag resolved for this run           |
| Injects an `ngrok` service         | Only when something publishes                             |

Everything else — `environment`, `healthcheck`, `command`, `depends_on`,
`networks`, named `volumes` — is passed through as written, already fully
interpolated.

Every field of both blocks is listed in
[Configuration Reference](./configuration-reference.md#x-minienv).
