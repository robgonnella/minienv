# Troubleshooting

When minienv fails, it names the field that was wrong and why, and it resolves
your whole configuration before deploying anything — so a rejected config never
leaves a half-applied environment behind. Read the message first; this page is
for what minienv does _not_ tell you.

## Nothing is exposed publicly

`NGROK_AUTHTOKEN` is not set. When it is missing, every `ngrok` block in your
config is discarded without a warning and the deploy still succeeds. Check this
before anything else.

## No URL table prints after a successful deploy

Printing the table is the last, cosmetic step, so failing to read the URLs back
is a warning rather than a failed deploy — the environment is already up.

The usual cause is an unset or invalid `NGROK_API_KEY`. That is a different
credential from `NGROK_AUTHTOKEN` and is optional: publishing does not need it,
and neither does teardown. Only the table does.

## The URL table shows `my-namespace-api` rather than `api`

That is the ngrok endpoint name, deliberately prefixed with the namespace so it
is unique on the account. It is also the name to look for in the ngrok
dashboard.

## A published URL changed after a redeploy

An endpoint with no reserved `ngrok.url` keeps its address only while the ngrok
agent survives. Changing the set of published services — or rotating the auth
token — replaces the agent, which reassigns every unreserved URL at once.

Pin the ones that matter with `ngrok.url`. A randomly assigned URL cannot be
re-claimed, so a reserved domain is the only way to hold an address. See
[Exposing Services](./exposing-services.md#a-stable-url).

## A new service is not published, or a removed one still is

Adding or removing an `ngrok` block replaces the agent, which also reassigns
every URL with no reserved `url`. If a URL looks stale, check whether the agent
actually restarted.

On Kubernetes:

```sh
kubectl get pods -n <namespace> -l app.kubernetes.io/name=ngrok
```

On Docker:

```sh
ssh <host> 'cd ~/.minienv/<namespace> && docker compose ps ngrok'
```

See [Exposing Services](./exposing-services.md#what-gets-published).

## The ngrok agent will not start

Check whether `ngrok.url` names a domain your account actually holds. The agent
refuses a domain that is not reserved, and names the one it could not claim in
its own logs.

On Kubernetes the whole deploy fails at its last step, and rolls back:

```sh
kubectl logs -n <namespace> -l app.kubernetes.io/name=ngrok
```

On Docker the rest of the project is already up and only the agent is unhealthy:

```sh
ssh <host> 'cd ~/.minienv/<namespace> && docker compose logs ngrok'
```

## Two developers cannot both publish the same service

Check whether `ngrok.url` is a hardcoded domain. Endpoint _names_ are prefixed
with the namespace, so lookups never cross environments — but a reserved domain
is committed to `compose.yml` and is therefore the same for everyone, and two
agents cannot claim one domain at once.

Interpolate the namespace into it:

```yaml
url: https://${MINIENV_NAMESPACE}-api.example.ngrok.app
```

## A redeploy did not pick up my rebuilt image

The image tag did not change, so neither target saw a reason to replace the
container. Use a tag that moves — `+git` appends the current short sha. See
[Images and Builds](./images-and-builds.md#tagging-per-commit-with-git).

On Kubernetes you can instead force a replacement on every deploy with
`recreate: true` on the service.

## A code change does not show up at all

Either the build was skipped — look for `missing required fields: skipping build
and push` in the output — or the target is reusing a cached image. A `+git` tag
gives every commit a unique one. See
[Images and Builds](./images-and-builds.md).

## A setting under a service extension has no effect

Four possibilities:

1. **The extension does not match the active deployer.** Each deployer reads
   only its own service extension; any other is ignored in full and silently,
   without an error. This is the first thing to check after switching targets.
2. The key is misspelled. Unknown extension keys are ignored at runtime. The
   [JSON schema](./getting-started.md#editor-autocomplete) would have flagged it
   in your editor — the schema is stricter than the runtime.
3. The key is one the target ignores. On Kubernetes a `deploymentType: job`
   ignores `replicas`, `service.*`, `volumes`, `volumeMounts`, `tolerations`,
   probes and `ngrok`.
4. The key is nested wrongly. The extension is mostly flat: `replicas` and
   `resources` sit directly under `x-minienv-k8s-service`, not under a `values`
   or `chart` key.

Each extension's accepted fields are listed in
[Configuration Reference](./configuration-reference.md).

## A compose setting has no effect

On Kubernetes, minienv reads a subset of compose: `entrypoint`, `volumes`,
`networks`, `restart`, `user` and others are ignored. On Docker, check whether
it is a bind mount or a `file:`-based secret, which cannot follow the project to
another host. Both are covered in
[what to change in your compose file](./configuration.md#what-to-change-in-your-compose-file).

## A health check never runs

A `curl` or `wget` healthcheck against anything other than `http://localhost`
produces no probes at all. Point it at `localhost`, using the **container**
port.

## A service has no Kubernetes Service

A service with no ports gets no Service and no ServiceAccount. Add a
`host:container` port mapping if you expected one.

## minienv cannot reach the remote host

For the `docker` deployer, the host key must already be in your
`~/.ssh/known_hosts`. minienv verifies it and offers no prompt or bypass, so
connect once by hand before the first deploy. Encrypted identity files are not
supported.

## Teardown failed and left containers behind

The directory holding your configuration and credentials is removed even when
teardown fails, so a retry starts clean. Containers or volumes may survive it;
check directly on the host with `docker ps`.
