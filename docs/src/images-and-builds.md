# Images and Builds

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
`image:` supplies whichever of them you leave unset:

So this is enough for a service you are not building:

```yaml
services:
  db:
    image: postgres:15 # repository: postgres, tag: 15
```

An untagged `image: myapp` resolves to `myapp:latest`, so it inherits the stale
image problem described under [Pull policy](#pull-policy).

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

Override when your cluster nodes differ from your laptop, or when you need both:

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

On a fixed tag like `latest`, set `Always` — otherwise the cluster keeps running
a stale cached image:

```yaml
x-minienv-k8s-service:
  image:
    pullPolicy: Always
```

## Private registries

```yaml
x-minienv-k8s-service:
  imagePullSecrets:
    - name: my-registry-creds
```

The secret must already exist in the target namespace; minienv does not create
it.
