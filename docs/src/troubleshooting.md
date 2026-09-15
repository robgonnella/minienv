# Troubleshooting

Read the error first — this page is for what minienv does _not_ tell you.

## Nothing is exposed publicly

`NGROK_AUTHTOKEN` is not set. When it is missing, every `ngrok` block is
discarded without a warning and the deploy still succeeds. Check this first.

## No URL table prints after a successful deploy

An unset or invalid `NGROK_API_KEY`. That is a different credential from
`NGROK_AUTHTOKEN` and only the table needs it, so the environment is already up.

## The URL table shows `my-namespace-api` rather than `api`

That is the ngrok endpoint name, prefixed with the namespace so it is unique on
the account. It is also the name to look for in the ngrok dashboard.

## A published URL changed after a redeploy

An endpoint with no reserved `ngrok.url` keeps its address only while the ngrok
agent survives. Changing the set of published services — or rotating the auth
token — replaces the agent, which reassigns every unreserved URL at once.

A reserved domain is the only way to hold an address — see
[Exposing Services](./exposing-services.md#a-stable-url).

## A new service is not published, or a removed one still is

If a URL looks stale, check whether the agent actually restarted.

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

Check whether `ngrok.url` is a hardcoded domain. It is committed to
`compose.yml` and is therefore the same for everyone, and two agents cannot
claim one domain at once. Interpolate the namespace into it:

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

Either the build was skipped or the target is reusing a cached image.

A service with a `build:` section is skipped when minienv cannot work out a
complete image reference — registry, tag, context and dockerfile. It logs
`missing required fields: skipping build and push`, then deploys whatever image
reference it does have. That is a warning, not an error.

For a cached image, a `+git` tag gives every commit a unique one. See
[Images and Builds](./images-and-builds.md).

## A setting under a service extension has no effect

1. **The extension does not match the active deployer.** Each deployer reads
   only its own, and ignores any other silently. Check this first after
   switching targets.
2. The key is misspelled. Unknown keys are ignored at runtime; the
   [JSON schema](./getting-started.md#editor-autocomplete) is stricter and would
   have flagged it in your editor.
3. The key is one the target ignores — see
   [what a job ignores](./jobs-and-dependencies.md#what-a-job-ignores).
4. The key is nested wrongly. The extension is mostly flat: `replicas` and
   `resources` sit directly under `x-minienv-k8s-service`.

## A compose setting has no effect

On Kubernetes, minienv reads a subset of compose: `entrypoint`, `volumes`,
`user` and others have an extension equivalent instead. On Docker, check whether
it is a bind mount or a `file:`-based secret, which cannot follow the project to
another host. Both are covered in
[supplying what compose does not carry](./configuration.md#supplying-what-compose-does-not-carry).

## A health check never runs

A `curl` or `wget` healthcheck against anything other than `http://localhost`
produces no probes at all. Point it at `localhost`, using the **container**
port.

## A service has no Kubernetes Service

A service with no ports gets no Service and no ServiceAccount. Add a port
mapping to compose `ports:` if you expected one.

## minienv cannot reach the remote host

For the `docker` deployer, the host key must already be in your
`~/.ssh/known_hosts`. minienv verifies it and offers no prompt or bypass, so
connect once by hand before the first deploy. Encrypted identity files are not
supported.

## Teardown failed and left containers behind

The directory holding your configuration and credentials is removed even when
teardown fails. Containers or volumes may survive it; check on the host with
`docker ps`.
