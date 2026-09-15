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

Use the service extension matching your target. The other one is silently
ignored.

Configure exactly one target in `x-minienv` — see
[Deployers](./deployers.md) for what each one does and what it needs.

## Per-service overrides

Optional — most services need nothing here. Every field is listed in the
[Configuration Reference](./configuration-reference.md).

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

## Supplying what compose does not carry

Your compose file stays as it is. minienv reads `image`, `build`, `ports`,
`command`, `environment`, `healthcheck` and `depends_on`; anything else a target
needs comes from the extension.

### Kubernetes

These compose keys are not read. Set the extension field when the deployed
service needs what they describe:

| Compose key          | Set in `x-minienv-k8s-service`         |
| -------------------- | -------------------------------------- |
| `entrypoint`         | `command`                              |
| `volumes`            | `volumes` and `volumeMounts`           |
| `secrets`, `configs` | `manifests`, mounted through `volumes` |
| `deploy.replicas`    | `replicas`                             |
| `labels`             | `podLabels`                            |
| `user`               | `securityContext`                      |

Storage is declared the Kubernetes way:

```yaml
x-minienv-k8s-service:
  volumes:
    - name: cache
      emptyDir: {}
  volumeMounts:
    - name: cache
      mountPath: /var/cache
```

A volume backed by a ConfigMap, Secret or PersistentVolumeClaim needs that
resource to exist — declare it in a file and list it under `manifests`.

`networks`, `restart`, `profiles`, `working_dir`, `extra_hosts` and `expose`
have no Kubernetes equivalent and are ignored.

### Docker

The project reaches the remote host as written, apart from what cannot follow it
there: bind mounts, whose host paths do not exist on that machine, and
`secrets` or `configs` declared with `file:`. Named volumes are kept.

## Next steps

- [Configuration Reference](./configuration-reference.md) — every field, with
  the per-field detail.
- [Images and Builds](./images-and-builds.md)
- [Exposing Services](./exposing-services.md)
- [Jobs and Dependencies](./jobs-and-dependencies.md)
