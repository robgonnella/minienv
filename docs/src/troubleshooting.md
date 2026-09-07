# Troubleshooting

## Configuration errors

**`no minienv extension config found in docker compose configs`**

Your compose file has no top-level `x-minienv` block. Add one — see
[Getting Started](./getting-started.md).

**`failed to find an active configuration for deployment. configure one of [k8s] in x-minienv extension field`**

`x-minienv` exists, but `k8s.context` or `k8s.namespace` is missing or empty.
Both are required.

**`failed to parse x-minienv top-level extension`**

A value in `x-minienv` has the wrong type — a list where a string belongs, for
example. Adding the [schema comment](./getting-started.md#editor-autocomplete)
catches this in your editor.

## Image errors

**`image.repository must be specified in service extension`**
**`image.tag must be specified in service extension`**

minienv could not work out an image reference. Either the service has no
`image:` in compose, or it has an untagged one like `image: myapp` — compose's
implicit `:latest` does not apply here.

Fix it with a tag (`image: myapp:latest`) or by setting both fields explicitly
under `x-minienv-k8s-service`.

**`failed to get short sha from git for image tag`**

You used a `+git` tag but `git` is unavailable, or the directory is not a git
repository with at least one commit.

## Port errors

**`invalid published port ...`**

A `ports:` entry is missing its host side:

```yaml
ports:
  - "8080" # error
  - "8080:8080" # correct
```

**`exposeServicePort must match a mapped port ...`**

`ngrok.port` does not match any of the service's ports. Use the **host** side of
the compose mapping — `8080` in `"8080:3000"`, not `3000`.

The message says `exposeServicePort`; the key you actually set is `ngrok.port`.
It is raised while extensions resolve, before any release is installed, so a
typo here never leaves half a project up.

## Deployment errors

**`invalid service dependency graph`**

Either a `depends_on` cycle, or a `depends_on` naming a service that does not
exist in the file.

**`invalid deploymentType`**

`deploymentType` accepts only `service` or `job`.

**A deploy times out and rolls back**

The default is 60 seconds per service. Raise it with `deploymentTimeout` — see
[Jobs and Dependencies](./jobs-and-dependencies.md#timeouts).

## Silent problems

These produce no error, which makes them the ones worth knowing about.

**Nothing is exposed publicly**

`NGROK_AUTHTOKEN` is not set. When it is missing, every `ngrok` block in your
config is discarded without a warning. Check this before anything else.

**`looking up published service urls requires NGROK_API_KEY to be set`**

A service has `ngrok` config but `NGROK_API_KEY` is unset. It is a different
credential from `NGROK_AUTHTOKEN`, and it is optional — the deploy still
succeeds and the services are still reachable. Only the URL table needs it, so
this arrives as a warning.

**No URL table prints after a successful deploy**

The table is the last, cosmetic step, so a failure reading the URLs back is a
warning rather than a failed deploy — the environment is already up. Check the
warning for the reason; an unset or invalid `NGROK_API_KEY` is the usual one.

**A redeploy did not pick up my rebuilt image**

The image tag did not change, so Helm sees an unchanged release and leaves the
pods alone. Either set `recreate: true` on the service, or use a tag that moves
— `+git` appends the current short sha. See
[Configuration Reference](./configuration-reference.md#recreate).

**A published URL changed after a redeploy**

An endpoint with no reserved `ngrok.url` keeps its address only while the ngrok
pod survives. Changing the set of published services replaces that pod, which
reassigns every unreserved URL at once. Pin the ones that matter with
`ngrok.url`. A randomly assigned URL cannot be re-claimed, so a reserved domain
is the only way to hold an address.

**The ngrok pod will not start, and the deploy rolled back**

Check whether `ngrok.url` names a domain your account actually holds. The agent
refuses a domain that is not reserved, and the ngrok release installs with
`--atomic`, so the whole deploy fails at its last step. `kubectl logs -n
<namespace> -l app.kubernetes.io/name=ngrok` names the domain it could not
claim.

**Two developers cannot both publish the same service**

Check whether `ngrok.url` is a hardcoded domain. Endpoint _names_ are prefixed
with the namespace, so lookups never cross environments, but a reserved domain
is committed to `compose.yml` and is therefore the same for everyone. Two agents
cannot claim one domain at once. Interpolate the namespace into it:

```yaml
url: https://${MINIENV_NAMESPACE}-api.example.ngrok.app
```

**The URL table shows `my-namespace-api` rather than `api`**

That is the ngrok endpoint name, which is deliberately prefixed with the
namespace so it is unique on the account. It is also the name to look for in the
ngrok dashboard.

**A new service is not published, or a removed one still is**

Adding or removing an `ngrok` block restarts the ngrok agent, which also
reassigns every URL that has no reserved `url`. If a URL looks stale, check
whether the `ngrok` pod actually restarted:

```sh
kubectl get pods -n <namespace> -l app.kubernetes.io/name=ngrok
```

**A setting under `x-minienv-k8s-service` has no effect**

Three possibilities:

1. The key is misspelled. Unknown extension keys are ignored at runtime. The
   [JSON schema](./getting-started.md#editor-autocomplete) would have flagged it
   in your editor — the schema is stricter than the runtime.
2. The service is `deploymentType: job`, and the key is one a job ignores
   (`replicas`, `service.*`, `volumes`, `volumeMounts`, `tolerations`, probes,
   `ngrok`).
3. The key is nested wrongly. Most of the extension is flat: `replicas` and
   `resources` sit directly under `x-minienv-k8s-service`, not under a `values`
   or `chart` key.

**A compose setting has no effect**

minienv reads a subset of compose. `entrypoint`, `volumes`, `networks`,
`restart`, `user`, and others are ignored — see
[what minienv ignores](./configuration.md#what-minienv-ignores).

**A code change does not show up in the environment**

Either the build was skipped (look for `missing required fields: skipping build
and push` in the output), or the cluster is reusing a cached image. Use a
`+git` tag so every commit produces a unique one — see
[Images and Builds](./images-and-builds.md#tagging-per-commit-with-git).

**A health check never runs**

A `curl` or `wget` healthcheck against anything other than `http://localhost`
produces no probes at all. Point it at `localhost`, using the **container**
port.

**A service has no Kubernetes Service**

A service with no ports gets no Service and no ServiceAccount. Add a
`host:container` port mapping if you expected one.
