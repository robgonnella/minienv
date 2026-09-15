# Jobs and Dependencies

## Run-to-completion jobs

On Kubernetes, set `deploymentType: job` to deploy a service as a Job instead of
a long-running Deployment. This is for work that finishes: migrations, seed
data, fixtures, smoke tests.

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

### What a job ignores

A job accepts the full extension schema, but several keys have no effect on one:
`replicas`, `service.create`, `service.type`, the three probes, and `ngrok` — a
job has no Service for an endpoint to route to.

Those keys are absent from a job's chart values, so a `manifests` file that
reads one — `.Values.replicas`, say — renders empty rather than failing.

## Ordering with `depends_on`

On Kubernetes, minienv deploys services in `depends_on` order and destroys them
in reverse, with independent services going up in parallel. On Docker your
`depends_on` block reaches the remote host as written, and compose orders the
project there.

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

On Kubernetes only the dependency **edges** are used, not the `condition:`
values — so `service_started` and `service_healthy` behave identically.

## Timeouts

Slow migrations and databases that take a while to initialize will hit the
default readiness window. Raise it for everything:

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

Values are duration strings: `30s`, `5m`, `1h30m`. If a deploy times out, the
release is rolled back.
