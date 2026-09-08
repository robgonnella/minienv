package config

// This package deliberately performs no environment lookups. Runtime values
// such as the ngrok auth token and the helm driver are read at the
// composition root (internal/command) and passed down explicitly, so that
// importing this package — including from a test binary — never pulls a
// credential into the process.

const HelmDefaultDeploymentTimeout = "60s"

const TopLevelExtension = "x-minienv"
const K8sServiceExtension = "x-minienv-k8s-service"

const NgrokImageRepo = "ngrok/ngrok"
const NgrokImageTag = "3.39.11-alpine"
const NgrokConfigMapName = "ngrok-config"
const NgrokConfigKey = "ngrok.yml"
const NgrokConfigVolMountPath = "/home/ngrok/.config/ngrok"
const NgrokSecretName = "ngrok-secret"
