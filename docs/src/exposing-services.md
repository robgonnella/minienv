# Exposing Services

A mini environment is only useful if someone can reach it. minienv exposes
services publicly through [ngrok](https://ngrok.com).

> **ngrok is the only exposure mechanism.** minienv never creates an Ingress, a
> LoadBalancer Service, or a PersistentVolumeClaim. If you need those, manage
> them outside minienv.

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
    x-minienv-k8s-service:
      ngrok:
        port: 8080
```

That is the whole setup. On deploy the service becomes reachable at a public
URL.

## How it is deployed

Every service with an `ngrok` block is published by **one ngrok deployment per
namespace**, installed as its own helm release named `ngrok` after all your
service releases have landed. Its config lists every published service at once
and routes each endpoint to that service's k8s Service.

Each endpoint is named `<namespace>-<service>`. The ngrok API reports every
endpoint on the whole account, so the namespace prefix is what keeps one
developer's environment from resolving another's URL. It is also the name shown
in the URL table and in the ngrok dashboard.

What this means day to day:

- Your own pods are unchanged — one container each. The agent is a separate
  `ngrok` pod alongside them.
- The agent is only restarted when the set of published endpoints actually
  changes. A deploy that changes nothing leaves it running, so URLs without a
  reserved domain survive redeploys.
- Adding or removing an `ngrok` block _does_ restart the agent, so every URL
  without a reserved domain is reassigned at that point.
- Removing the last `ngrok` block uninstalls the release, and `minienv down`
  removes it along with everything else.

## Which port number to use

`ngrok.port` must match one of the service's resolved ports. When the ports came
from a compose mapping, use the **host** side:

```yaml
ports:
  - "8080:3000" # host 8080, container 3000
x-minienv-k8s-service:
  ngrok:
    port: 8080 # the host side
```

Using the container port here is an error, not a silent fallback. The whole
deploy is rejected before anything is installed. See
[Troubleshooting](./troubleshooting.md).

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

> **The domain has to already be reserved.** minienv passes this value to the
> ngrok agent as-is, and the agent refuses a domain the account does not hold —
> which fails the deploy. Every account has at least one reserved static domain,
> including a free one; a paid plan is what buys you more than one and lets you
> choose the name.

> **On a shared ngrok account, interpolate the namespace into it.** This value
> is committed to `compose.yml`, so a hardcoded domain is the _same_ domain for
> everyone on the team, and two agents cannot claim one domain at once:
>
> ```yaml
> url: https://${MINIENV_NAMESPACE}-api.example.ngrok.app
> ```
>
> Compose interpolates extension values the same way it does
> `namespace: $MINIENV_NAMESPACE`, so each developer gets their own. The domain
> still has to be reserved in ngrok — a wildcard reservation covers the whole
> pattern.

Without a reserved domain the URL survives a redeploy that changes nothing, but
adding or removing any published service replaces the ngrok pod and reassigns
every unreserved URL at once.

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

There is a second, optional credential alongside the auth token:

```sh
export NGROK_API_KEY=your_api_key_here
```

minienv uses it to look up the URL each endpoint is serving on, so it can print
the table below. That is all it does — publishing does not need it.

> **`NGROK_API_KEY` is optional.** Without it a deploy that publishes still
> succeeds and everything is still reachable; you get a warning instead of the
> URL table. `minienv down` never needs it either — teardown does not look
> anything up.

```
====== Published Service URLs ======
Service Name   URL
--             ----
api            https://quiet-mesa-1234.ngrok.app
web            https://my-branch.example.ngrok.app
```

Two separate credentials, doing two different jobs — only one is required:

| Variable          | What it does                                                        |
| ----------------- | ------------------------------------------------------------------- |
| `NGROK_AUTHTOKEN` | **Publishes.** Without it every `ngrok` block is silently discarded |
| `NGROK_API_KEY`   | **Prints the URL table.** Optional; without it the table is skipped |

Once the deploy itself has succeeded, a failure printing the table is only a
warning — the environment is already up by then, so it is never failed over the
report at the end.

> The lookup matches ngrok endpoints by name against your service names, and the
> ngrok API lists every endpoint on the **account**. If two people share one
> ngrok account and both have a service called `api`, the table can show the
> other environment's URL.

## Things to expect

- ngrok configuration has **no effect on `deploymentType: job`**. A job renders
  no k8s Service, so there is nothing for an endpoint to route to. minienv logs
  a warning and publishes nothing for it.
- The same applies to a service marked `skip: true` — it is never installed, so
  it is never published.
