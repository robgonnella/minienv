# Images and Builds

Every service resolves to one image: a repository and a tag. The `image` fields
below are the same under `x-minienv-k8s-service` and `x-minienv-docker-service`;
the examples use the first.

## Building from source

Any service with a `build:` section is built and pushed before the deploy
starts:

```yaml
services:
  api:
    build: .
    x-minienv-k8s-service:
      image:
        repository: myorg/api
        tag: v1
```

## Where the image name comes from

minienv needs a repository and a tag. The extension fields win, and compose
`image:` supplies whichever of them you leave unset, so this is enough for a
service you are not building:

```yaml
services:
  db:
    image: postgres:15 # repository: postgres, tag: 15
```

An untagged `image: myapp` resolves to `myapp:latest` — see
[Fixed tags](#fixed-tags).

> **A digest reference is rejected.** `image: myapp@sha256:...` fails the
> deploy — set `image.repository` and `image.tag` in the extension instead.

## Tagging per commit with `+git`

The literal string `+git` in a tag is replaced with the current short commit
SHA:

```yaml
x-minienv-k8s-service:
  image:
    repository: myorg/api
    tag: +git
```

Every commit gets a unique tag, so the target always pulls the image you just
built instead of a cached one. It works as a suffix too — `tag: v1-+git`
produces something like `v1-a1b2c3d`.

## Platforms

Override when the target's architecture differs from your laptop, or when you
need both:

```yaml
x-minienv-k8s-service:
  image:
    repository: myorg/api
    tag: +git
    platforms:
      - linux/amd64
      - linux/arm64/v8
```

## Fixed tags

A tag rebuilt in place — `latest`, `dev` — names an image the target may
already hold, so a redeploy can keep running the old one. `+git` sidesteps this
by changing the tag every commit. Where it is not an option:

### Kubernetes

Set the pull policy to `Always`:

```yaml
x-minienv-k8s-service:
  image:
    pullPolicy: Always
```

Or set `recreate: true`, which replaces the pods on every deploy even when
nothing in the release changed.

### Docker

The remote host pulls with `docker compose up -d`, which by default keeps an
image it already has. Set compose `pull_policy` on the service; it reaches the
host as written:

```yaml
services:
  api:
    image: myorg/api:latest
    pull_policy: always
```

## Private registries

### Kubernetes

```yaml
x-minienv-k8s-service:
  imagePullSecrets:
    - name: my-registry-creds
```

The secret must already exist in the target namespace; minienv does not create
it.

### Docker

The remote host's Docker daemon pulls the image, so it must already be logged
in to the registry. minienv does not log it in.
