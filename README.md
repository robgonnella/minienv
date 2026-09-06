# minienv

Effortless remote mini environments generated directly from docker compose
config.

You already describe your stack in docker compose config. minienv reads that
config, plus small extension blocks, and deploys it to a remote environment.
Those same extensions can optionally enable public endpoints for the services in
your config, including various authentication mechanisms, so you can collaborate
with others on in-flight work.

## Install

```sh
go install github.com/robgonnella/minienv/cmd/minienv@latest
```

## Usage

Add an `x-minienv` block to your compose file:

```yaml
x-minienv:
  k8s:
    context: minikube
    namespace: my-feature-branch

services:
  hello:
    image: rgonnella/demo-hello:latest
    ports:
      - "8080:8080"
```

Then:

```sh
minienv deploy     # bring it up
minienv destroy    # tear it down
```

Add `--dry-run` to either to preview without pushing images or touching the
cluster.

## Documentation

Full documentation lives in [`docs/`](./docs) and covers configuration, image
builds, public URLs via ngrok, jobs and dependency ordering, and a complete
field reference.

Build it locally with [mdBook](https://rust-lang.github.io/mdBook/):

```sh
just docs-serve
```

## Deployers

Kubernetes is currently the only supported deployment target. More are planned.
