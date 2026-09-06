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

That is the whole setup. On deploy, an ngrok sidecar container is added to the
pod and the service becomes reachable at a public URL.

## Which port number to use

`ngrok.port` must match one of the service's resolved ports. When the ports came
from a compose mapping, use the **host** side:

```yaml
ports:
  - "8080:3000"    # host 8080, container 3000
x-minienv-k8s-service:
  ngrok:
    port: 8080     # the host side
```

Using the container port here is an error, not a silent fallback. See
[Troubleshooting](./troubleshooting.md).

## A stable URL

By default ngrok assigns a random URL that changes on every deploy. Pin a
reserved domain instead:

```yaml
x-minienv-k8s-service:
  ngrok:
    port: 8080
    url: https://my-branch.example.ngrok.app
```

Useful when the URL goes in a pull request description, or when an external
service needs a fixed webhook target.

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

## Things to expect

- Pods for an exposed service run **two containers** — yours and the ngrok
  sidecar. An extra container in `kubectl get pod` output is normal.
- ngrok configuration has **no effect on `deploymentType: job`**. Jobs are not
  exposed.
