package config

import "os"

var NGROK_AUTHTOKEN = os.Getenv("NGROK_AUTHTOKEN")

const TOP_LEVEL_EXTENSION = "x-minienv"
const SERVICE_K8S_EXTENSION = "x-minienv-k8s-service"
const NGROK_IMAGE = "ngrok/ngrok:3.39.11-alpine"
const NGROK_CONFIG_MAP_NAME = "ngrok-config"
const NGROK_TRAFFIC_POLICY_CONFIG_KEY = "traffic-policy.yml"
const NGROK_CONFIG_VOL_MOUNT_PATH = "/etc/ngrok"
const NGROK_SECRET_NAME = "ngrok-secret"
