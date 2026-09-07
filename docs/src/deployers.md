# Deployers

minienv separates _reading your compose config_ from _deploying it somewhere_.
The reading half is fixed; the deploying half is pluggable.

## Available today

**Kubernetes is the only deployer currently implemented.** It deploys each
compose service as its own Helm release, using a chart minienv generates
internally — there is no chart to maintain in your repository, and no `helm`
binary to install.

It activates when you set both required fields:

```yaml
x-minienv:
  k8s:
    context: minikube
    namespace: my-branch
```

If neither is set, minienv has no target and stops with:

```
failed to find an active configuration for deployment. configure one of [k8s] in x-minienv extension field
```

## More to come

Additional deployment targets are planned.

The `x-minienv` block is keyed by target — `k8s` today — so new deployers arrive
as new keys alongside it. Existing configurations keep working unchanged when
that happens.

Exactly one target may be active at a time. Once other deployers exist,
configuring two will be an error rather than a silent choice between them.
