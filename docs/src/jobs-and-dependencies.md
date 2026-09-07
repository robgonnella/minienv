# Jobs and Dependencies

## Run-to-completion jobs

Set `deploymentType: job` to deploy a service as a Kubernetes Job instead of a
long-running Deployment. This is for work that finishes: migrations, seed data,
fixtures, smoke tests.

```yaml
services:
  migrate:
    image: myorg/api:v1
    command: ["./manage", "migrate"]
    x-minienv-k8s-service:
      deploymentType: job
```

Every deploy creates a **fresh Job**, so re-running a migration works without
manually deleting the previous one.

The only valid values are `service` (the default) and `job`.

### What a job ignores

A job accepts the full extension schema, but several keys have no effect on one:
`replicas`, `service.create`, `service.type`, `volumes`, `volumeMounts`,
`tolerations`, the three probes, and `ngrok`.

Ports are still read — they define the container's declared ports — but no
Kubernetes Service is created for a job. That is also why `ngrok` is ignored:
an endpoint's upstream is a Service, and a job has none. minienv logs a warning
and publishes nothing rather than creating a URL that cannot route.

The Job resource itself is named with a random suffix, because a Job's
`spec.template` and `spec.selector` are immutable and an upgrade cannot patch
one in place — every deploy renders a new Job. The helm release is named after
the service, so `minienv down` finds and removes it.

## Ordering with `depends_on`

minienv deploys services in `depends_on` order and destroys them in reverse.
Independent services deploy in parallel, up to five at a time.

```yaml
services:
  api:
    image: myorg/api:v1
    ports:
      - "8080:8080"
    depends_on:
      migrate:
        condition: service_completed_successfully

  migrate:
    image: myorg/api:v1
    command: ["./manage", "migrate"]
    x-minienv-k8s-service:
      deploymentType: job
    depends_on:
      postgres:
        condition: service_healthy

  postgres:
    image: postgres:15
    environment:
      POSTGRES_PASSWORD: secret
    ports:
      - "5432:5432"
    healthcheck:
      test: pg_isready -U postgres -d postgres
```

This deploys `postgres`, waits for it, runs `migrate` to completion, then
deploys `api`.

### How the conditions are treated

minienv uses the dependency **edges** to build a deploy order. It does not
interpret the `condition:` values themselves.

In practice each service still waits for the one before it, because every
release is deployed with Helm waiting for pods to become ready and for jobs to
complete before moving on. The practical consequence is that
`condition: service_started` behaves the same as `condition: service_healthy` —
both simply create an edge.

Cycles, and a `depends_on` naming a service that does not exist, are caught
before anything is deployed.

## Timeouts

Each service gets 60 seconds to become ready by default. Slow migrations and
databases that take a while to initialize will hit that limit.

Raise it for everything:

```yaml
x-minienv:
  k8s:
    context: minikube
    namespace: my-branch
    deploymentTimeout: 5m
```

Or for a single service, which is usually the better fit:

```yaml
services:
  migrate:
    x-minienv-k8s-service:
      deploymentType: job
      deploymentTimeout: 10m
```

Values are Go duration strings: `30s`, `5m`, `1h30m`.

A service-level `deploymentTimeout` overrides the top-level one. If a deploy
times out, the release is rolled back.
