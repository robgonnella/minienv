# Exposing Services

A mini environment is only useful if someone can reach it. minienv exposes
services publicly through [ngrok](https://ngrok.com).

> **ngrok is the only exposure mechanism.** minienv never creates an Ingress, a
> LoadBalancer Service, or a PersistentVolumeClaim. If you need those, manage
> them outside minienv or via `manifests` in the k8s service extension.

## Setup

**1. Set your auth token.** minienv reads `NGROK_AUTHTOKEN` from the
environment:

```sh
export NGROK_AUTHTOKEN=your_token_here
```

> **If `NGROK_AUTHTOKEN` is not set, all ngrok configuration is silently
> discarded.** The deploy succeeds, nothing is exposed, and no error is printed.
> This is by far the most common reason for "why is nothing public". Check this
> first.

**2. Pick a port to expose:**

```yaml
services:
  api:
    image: myorg/api:v1
    ports:
      - "8080:8080"
    x-minienv-k8s-service: # or x-minienv-docker-service
      ngrok:
        port: 8080
```

On deploy the service becomes reachable at a public URL. The examples below use
`x-minienv-k8s-service`; use `x-minienv-docker-service` for the docker target.

## What gets published

One ngrok agent serves every published service in a namespace.

Each endpoint is named `<namespace>-<service>` — the name shown in the URL table
and the ngrok dashboard, and the prefix is what keeps one developer's
environment from resolving another's URL on a shared account.

The agent is replaced only when the published set or the auth token changes.
That is what decides whether a URL survives a redeploy — an unreserved one is
reassigned every time the agent is replaced.

## Which port number to use

`ngrok.port` is the **container** side of a compose mapping — the port your
process listens on. The host side plays no part in it.

```yaml
ports:
  - "8080:3000" # host 8080, container 3000
x-minienv-k8s-service: # or x-minienv-docker-service
  ngrok:
    port: 3000
```

## A stable URL

By default ngrok assigns a random URL, and a randomly assigned URL cannot be
re-claimed once the agent serving it goes away — not on any plan. A stable URL
means a domain **reserved on your ngrok account**, set explicitly:

```yaml
x-minienv-k8s-service:
  ngrok:
    port: 8080
    url: https://my-branch.example.ngrok.app
```

Useful when the URL goes in a pull request description, or when an external
service needs a fixed webhook target.

> **The domain has to already be reserved.** The agent refuses one the account
> does not hold, which fails the deploy. Every account has at least one static
> domain, including a free one; a paid plan buys more and lets you name them.

> **On a shared ngrok account, interpolate the namespace into it.** This value
> is committed to `compose.yml`, so a hardcoded domain is the _same_ domain for
> everyone on the team, and two agents cannot claim one domain at once:
>
> ```yaml
> url: https://${MINIENV_NAMESPACE}-api.example.ngrok.app
> ```
>
> A wildcard reservation covers the whole pattern.

## Traffic policy

`trafficPolicy` takes an ngrok `on_http_request` policy — for adding
authentication in front of the environment, rewriting headers, or restricting
access by IP:

```yaml
x-minienv-k8s-service:
  ngrok:
    port: 8080
    trafficPolicy: |
      on_http_request:
        - actions:
            - type: basic-auth
              config:
                credentials:
                  - "reviewer:hunter2"
```

To apply one policy to every service, set it once at the top level. Services
that define their own override it:

```yaml
x-minienv:
  ngrok:
    trafficPolicy: |
      on_http_request:
        - actions:
            - type: basic-auth
              ...
```

## The API key

`NGROK_API_KEY` is a second, optional credential, used only to print the URL
table after a deploy:

```sh
export NGROK_API_KEY=your_api_key_here
```

```
====== Published Service URLs ======
Service Name   URL
--             ----
api            https://quiet-mesa-1234.ngrok.app
web            https://my-branch.example.ngrok.app
```
