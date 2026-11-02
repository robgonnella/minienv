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

| Resource       | When                                                                                              |
| -------------- | ------------------------------------------------------------------------------------------------- |
| Deployment     | `deploymentType: service` (the default)                                                           |
| Job            | `deploymentType: job`                                                                             |
| Service        | `deploymentType: service` and `service.create` is true                                            |
| ServiceAccount | `serviceAccount.create` is true                                                                   |
| ConfigMap      | `configMapFrom` names at least one file                                                           |
| Secret         | compose `environment` or an `env` value takes a value from the local environment or an `env_file` |

Once per namespace:

| Resource                                                       | When                            |
| -------------------------------------------------------------- | ------------------------------- |
| `ngrok` release: Deployment, ConfigMap, Secret, ServiceAccount | any service publishes via ngrok |

The target namespace is created on deploy if it does not already exist. On
destroy it is left in place, unless `removeNamespaceOnDestroy` is `true` — then
the whole namespace is deleted, including anything in it that minienv did not
create.

Compose `environment` is split by where each value came from. A value written
literally in the compose file is rendered inline on the container. A value
taken from the local environment — `${VAR}`, or the bare `- VAR` form — or
from an `env_file` goes into a Secret named after the service and reaches the
container through `envFrom`, so it never appears in the Deployment. An `env`
entry under `x-minienv-k8s-service` whose `value` is `${VAR}` is placed the
same way.

Nothing else is derived from the compose file — no Ingress,
PersistentVolumeClaim, HorizontalPodAutoscaler or PodDisruptionBudget.
Anything else a service needs is declared explicitly, with `configMapFrom` or
`manifests`.

## Docker

Deploys the whole project to a single remote host as one `docker compose`
project, over a transport. The host runs `docker` and its compose plugin;
exactly one block under `transport` says how minienv reaches it.

### `transport.ssh`

Connects over SSH with a private key:

```yaml
x-minienv:
  docker:
    namespace: my-branch
    transport:
      ssh:
        host: dev-box.example.com
        identity: ~/.ssh/id_ed25519
```

The host key must already be in `~/.ssh/known_hosts` on the machine running
minienv. It verifies against that file and offers no prompt or bypass, so
connect once by hand before the first deploy.

### `transport.awsSsm`

Runs each command on the instance through an AWS Systems Manager Session
Manager command session, as the instance's `ssm-user`. No key, no sshd, no
inbound port:

```yaml
x-minienv:
  docker:
    namespace: my-branch
    transport:
      awsSsm:
        target: i-0123456789abcdef0
        profile: dev
```

What it needs:

- The instance registered with Systems Manager, with `ssm-user` able to run
  `docker` (in the `docker` group). The project lands under
  `/home/ssm-user/.minienv/<namespace>`.
- `tar`, `gzip` and `base64` in `ssm-user`'s shell — part of the base install
  on Amazon Linux, Ubuntu, Debian, RHEL and Alpine. `copy` entries arrive as
  a compressed archive, and a missing `tar` fails that step with a message
  saying so rather than extracting.
- `session-manager-plugin` 1.2.497.0 or newer on `PATH`. minienv starts each
  session itself and hands it to the plugin; the session token travels in
  the plugin's environment, never in its arguments.
- Credentials allowed `ssm:StartSession` on the target with the
  `AWS-StartNonInteractiveCommand` document, and `ssm:TerminateSession`.
  Region and credentials resolve the way the AWS SDK resolves them; `profile`
  names a shared-config profile to resolve them from.

Files reach the instance as `base64` inside the session's command, in chunks,
and directories as a compressed `tar` the same way. That means their
contents — the rendered `compose.yml`, and the `.env` holding your ngrok
token when something publishes — pass through the `StartSession` API as
parameters. Treat whatever records those parameters, such as CloudTrail or
session logging, as holding them.

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
