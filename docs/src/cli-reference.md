# CLI Reference

## Commands

| Command           | Aliases | Description                            |
| ----------------- | ------- | -------------------------------------- |
| `minienv deploy`  | `up`    | Brings up your remote mini environment |
| `minienv destroy` | `down`  | Tears it down                          |
| `minienv version` | —       | Prints version info                    |

None of them take positional arguments.

## Flags

All flags are global and work on every command.

| Flag                  | Short | Default | Description                                                               |
| --------------------- | ----- | ------- | ------------------------------------------------------------------------- |
| `--file`              | `-f`  | —       | Compose configuration file. Repeatable. Same as `docker compose -f`       |
| `--project-directory` | —     | —       | Alternate working directory. Same as `docker compose --project-directory` |
| `--project-name`      | `-p`  | —       | Project name. Same as `docker compose -p`                                 |
| `--dry-run`           | —     | `false` | Build images without pushing, and run the deploy without applying it      |

The first three behave exactly as they do in `docker compose`, because minienv
delegates file discovery to the same library.

## Environment variables

| Variable          | Effect                                                                                                                |
| ----------------- | --------------------------------------------------------------------------------------------------------------------- |
| `NGROK_AUTHTOKEN` | Enables ngrok. **Unset means all ngrok config is silently ignored** — see [Exposing Services](./exposing-services.md) |
| `NGROK_API_KEY`   | **Optional.** Only used to print the published URL table after a deploy. Not needed to publish, or by `minienv down`  |
| `HELM_DRIVER`     | Helm storage backend. Defaults to `secret`                                                                            |

Standard compose environment handling also applies: `COMPOSE_FILE`,
`COMPOSE_PROJECT_NAME`, `.env` file loading, and `${VAR}` interpolation inside
the compose file all work as usual.

That last one is the easiest way to give each developer or branch its own
namespace:

```yaml
x-minienv:
  k8s:
    context: minikube
    namespace: $MINIENV_NAMESPACE
```

```sh
MINIENV_NAMESPACE=alice-feature-x minienv deploy
```

## Examples

```sh
# Deploy using the default compose file
minienv deploy

# Deploy with an override file
minienv deploy -f compose.yml -f compose.prod.yml

# See what would happen, without pushing images or touching the cluster
minienv deploy --dry-run

# Tear down
minienv destroy
```
