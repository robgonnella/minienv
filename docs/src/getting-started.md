# Getting Started

## Prerequisites

| Requirement                                                | When you need it                               |
| ---------------------------------------------------------- | ---------------------------------------------- |
| `docker` with `buildx`                                     | Only for services that have a `build:` section |
| `git`                                                      | Only if you use a `+git` image tag             |
| A reachable Kubernetes context                             | Only for the `k8s` deployer                    |
| An SSH-reachable host with `docker` and its compose plugin | Only for the `docker` deployer                 |

Helm is built into minienv as a library — you do **not** need the `helm` binary
installed. The `docker` deployer likewise needs no extra client: it reaches the
remote host over SSH directly.

For the `docker` deployer, the remote host must already be in your
`~/.ssh/known_hosts`. minienv verifies the host key and offers no prompt or
bypass, so connect once by hand first. Encrypted identity files are not
supported.

## Install

```sh
go install github.com/robgonnella/minienv/cmd/minienv@latest
```

Check it:

```sh
minienv version
```

## Your first mini environment

Add a top-level `x-minienv` block to your compose file. This is the only thing
minienv requires:

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

`context` and `namespace` are both required. Setting them is what tells minienv
to deploy to Kubernetes.

Deploy it:

```sh
minienv deploy
```

Tear it down:

```sh
minienv destroy
```

Both commands accept `up` and `down` as aliases.

## Preview without deploying

`--dry-run` previews a deploy without changing the target:

```sh
minienv deploy --dry-run
```

It is not, however, entirely offline. Images are still built locally with
`buildx` — they are simply not pushed.

**Kubernetes.** The cluster is still contacted, read-only: reachability checks,
API discovery, and reading existing release state. Nothing is applied.

> **A `k8s` dry run needs a reachable cluster.** It changes nothing, but it is
> not a way to check a config without one.

**Docker.** Nothing remote happens at all. minienv logs the paths it would write
and opens no SSH session.

## File discovery

minienv finds your compose files exactly the way `docker compose` does —
including default file names, `COMPOSE_FILE`, `.env` loading, and variable
interpolation. The same flags work too:

```sh
minienv deploy -f compose.yml -f compose.override.yml
```

This means you can give every developer their own namespace with nothing but an
environment variable:

```yaml
x-minienv:
  k8s:
    context: minikube
    namespace: $MINIENV_NAMESPACE
```

## Editor autocomplete

minienv ships a JSON Schema that wraps the official compose spec and adds its
own extension fields. Point at it from the top of your compose file:

```yaml
# $schema: https://raw.githubusercontent.com/robgonnella/minienv/refs/heads/main/schema/schema.json
```

Editors that honor this comment will autocomplete and validate the `x-minienv`
blocks.

This is worth doing: the schema is **stricter than the runtime**. It rejects
unknown keys, whereas at deploy time a misspelled extension key is silently
ignored — your setting simply has no effect, with no error to tell you why.

## Next steps

- [Configuration](./configuration.md) — what minienv reads from your compose
  file, and what it ignores.
- [Exposing Services](./exposing-services.md) — make the environment reachable
  for review.
