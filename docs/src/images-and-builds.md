# Images and Builds

## Building from source

Any service with a `build:` section is built and pushed automatically before the
deploy starts, using `docker buildx bake`. Nothing extra is required:

```yaml
services:
  api:
    build: .
    x-minienv-k8s-service:
      image:
        repository: myorg/api
        tag: v1
```

Under `--dry-run`, images are built but not pushed.

## Where the image name comes from

minienv needs a repository and a tag. It looks in two places, in order:

1. `image.repository` and `image.tag` in `x-minienv-k8s-service`.
2. The compose `image:` field, split on the first `:`.

So this is enough for a service you are not building:

```yaml
services:
  db:
    image: postgres:15 # repository: postgres, tag: 15
```

> **An untagged image is an error.** `image: myapp` gives neither a repository
> nor a tag, and the deploy fails. Compose's implicit `:latest` does not apply —
> write `image: myapp:latest` or set the extension fields explicitly.

## Tagging per commit with `+git`

The literal string `+git` in a tag is replaced with the current short commit
SHA:

```yaml
x-minienv-k8s-service:
  image:
    repository: myorg/api
    tag: +git
```

This is the idiomatic way to deploy a branch: every commit gets a unique tag, so
the cluster always pulls the image you just built instead of a cached one. It
works as a suffix too — `tag: v1-+git` produces something like `v1-a1b2c3d`.

Requires `git` on `PATH` and a repository with at least one commit.

## Platforms

Images build for `linux/amd64` by default. Override when your cluster nodes
differ from your laptop, or when you need both:

```yaml
x-minienv-k8s-service:
  image:
    repository: myorg/api
    tag: +git
    platforms:
      - linux/amd64
      - linux/arm64/v8
```

## Pull policy

Defaults to `IfNotPresent`.

```yaml
x-minienv-k8s-service:
  image:
    pullPolicy: Always
```

If you use a fixed tag like `latest` rather than `+git`, set `Always` — or the
cluster will keep running a stale cached image.

## Private registries

```yaml
x-minienv-k8s-service:
  imagePullSecrets:
    - name: my-registry-creds
```

The secret must already exist in the target namespace; minienv does not create
it.

## Build skipped unexpectedly

If a service has a `build:` section but minienv cannot work out a complete
image reference — registry, tag, context, and dockerfile — it logs:

```
missing required fields: skipping build and push
```

and continues, deploying whatever image reference it does have. This is a
warning, not an error, so watch for it if a code change does not appear in the
deployed environment.
