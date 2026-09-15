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

Or target a remote docker host instead of a cluster:

```yaml
x-minienv:
  docker:
    namespace: my-feature-branch
    transport:
      ssh:
        host: dev-box.example.com
        identity: ~/.ssh/id_ed25519
```

Add `--dry-run` to either command to preview without pushing images or changing
the target.

## Documentation

Full documentation lives in [`docs/`](./docs) and covers configuration, image
builds, public URLs via ngrok, jobs and dependency ordering, and a complete
field reference.

Build it locally with [mdBook](https://rust-lang.github.io/mdBook/):

```sh
just docs-serve
```

## Deployers

Deploy targets are configured under `x-minienv`:

- **`k8s`** — deploys each compose service as its own Helm release, using a
  chart minienv generates internally. No chart to maintain, no `helm` binary.
- **`docker`** — deploys the whole project to a single remote host over SSH, as
  one `docker compose` project under `~/.minienv/<namespace>`.

Exactly one may be configured. See [Deployers](./docs/src/deployers.md).
