package config

// This package deliberately performs no environment lookups. Runtime values
// such as the ngrok auth token and the helm driver are read at the
// composition root (internal/command) and passed down explicitly, so that
// importing this package — including from a test binary — never pulls a
// credential into the process.

var HELM_DEFAULT_DEPLOYMENT_TIMEOUT = "60s"

const TOP_LEVEL_EXTENSION = "x-minienv"
const K8S_SERVICE_EXTENSION = "x-minienv-k8s-service"

const NGROK_IMAGE = "ngrok/ngrok:3.39.11-alpine"
const NGROK_CONFIG_MAP_NAME = "ngrok-config"
const NGROK_CONFIG_KEY = "ngrok.yml"
const NGROK_CONFIG_VOL_MOUNT_PATH = "/home/ngrok/.config/ngrok"
const NGROK_SECRET_NAME = "ngrok-secret"
