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
    x-minienv-k8s-service: # or x-minienv-docker-service
      ngrok:
        port: 8080
```

That is the whole setup. On deploy the service becomes reachable at a public
URL.

> Put the `ngrok` block under the service extension matching your deployer:
> `x-minienv-k8s-service` or `x-minienv-docker-service`. The examples below use
> the former. See [Which port number to use](#which-port-number-to-use) for the
> port value.

## What gets published

One ngrok agent serves every published service in a namespace, and runs
alongside your containers without changing them.

Each endpoint is named `<namespace>-<service>`. That is the name shown in the
URL table and in the ngrok dashboard, and the namespace prefix is what keeps one
developer's environment from resolving another's URL on a shared account.

The agent is replaced only when the published set or the auth token changes,
which is what decides whether a URL survives:

- A deploy that changes neither leaves URLs without a reserved domain alone.
- Adding or removing any `ngrok` block reassigns every unreserved URL at once.
- Removing the last `ngrok` block removes the agent, and `minienv destroy`
  removes it along with everything else.

## Which port number to use

`ngrok.port` must match one of the service's resolved ports. Which number that
is depends on the deployer you configured.

### Kubernetes

Use the **host** side of a compose mapping. The endpoint routes through a k8s
Service, and the host side is what becomes the Service port.

```yaml
ports:
  - "8080:3000" # host 8080, container 3000
x-minienv-k8s-service:
  ngrok:
    port: 8080
```

### Docker

Use the **container** side of a compose mapping. The agent shares the compose
network and dials the container directly, so no published port is in the path.

```yaml
ports:
  - "8080:3000" # host 8080, container 3000
x-minienv-docker-service:
  ngrok:
    port: 3000
```

### When it does not match

Naming a port the service did not resolve is an error, not a silent fallback.
The whole deploy is rejected before anything is installed, and the message lists
the ports that _were_ on offer:

```
ngrok port 8080 matches no port for this service: expected one of [3000]
```

See [Troubleshooting](./troubleshooting.md).

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
adding or removing any published service replaces the agent and reassigns every
unreserved URL at once.

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

minienv uses it to print the URL table after a deploy. That is all it does:

```
====== Published Service URLs ======
Service Name   URL
--             ----
api            https://quiet-mesa-1234.ngrok.app
web            https://my-branch.example.ngrok.app
```

Two credentials, two jobs — only one is required:

| Variable          | What it does                                                        |
| ----------------- | ------------------------------------------------------------------- |
| `NGROK_AUTHTOKEN` | **Publishes.** Without it every `ngrok` block is silently discarded |
| `NGROK_API_KEY`   | **Prints the URL table.** Optional; without it the table is skipped |

Without the API key a publishing deploy still succeeds and everything is still
reachable — you get a warning instead of the table. Teardown never needs it.

## Things to expect

- ngrok configuration has **no effect on a service marked `skip: true`**. It is
  never deployed, so there is nothing for the endpoint's upstream to reach.
  minienv logs a warning and publishes nothing for it.
- On Kubernetes, the same applies to **`deploymentType: job`**. A job renders no
  k8s Service, so there is nothing for an endpoint to route to.
