#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE_SOURCE="$ROOT_DIR/deploy/master/docker-compose.yml"
DEPLOY_DIR="${FRP_PANEL_DEPLOY_DIR:-/opt/frp-panel}"
TAG=""
BUILD_IMAGE="false"
API_HOST=""
RPC_HOST=""
CLIENT_API_URL=""
CLIENT_RPC_URL=""
WEB_PORT="9000"
RPC_PORT="9001"
SKIP_PULL="false"

usage() {
  cat <<'EOF'
Initialize or update the FRP-Panel Master Docker Compose deployment.

Usage:
  sudo scripts/deploy-master.sh --api-host <domain-or-ip> [options]

Options:
  --dir <path>             Deployment directory (default: /opt/frp-panel)
  --tag <tag>              Image tag from scripts/build-image.sh (default: alpha)
  --build                  Build and load this tag locally before deploying (requires --tag)
  --api-host <host>        Public host used for the Master API and first Server
  --rpc-host <host>        Public host used for Master RPC (default: API host)
  --client-api-url <url>   URL nodes use for the API (default: http://<api-host>:<web-port>)
  --client-rpc-url <url>   URL nodes use for RPC (default: grpc://<rpc-host>:<rpc-port>)
  --web-port <port>        Host port published for the web UI (default: 9000)
  --rpc-port <port>        Host port published for RPC (default: 9001)
  --skip-pull              Use an image already loaded into the local Docker engine
  -h, --help               Show this help

For a reverse proxy, pass its externally reachable URLs, for example:
  --client-api-url https://panel.example.com
  --client-rpc-url wss://panel.example.com
EOF
}

while (($#)); do
  case "$1" in
    --dir|--tag|--api-host|--rpc-host|--client-api-url|--client-rpc-url|--web-port|--rpc-port)
      [[ $# -ge 2 ]] || { echo "Missing value for $1" >&2; exit 2; }
      key="$1"
      value="$2"
      case "$key" in
        --dir) DEPLOY_DIR="$value" ;;
        --tag) TAG="$value" ;;
        --api-host) API_HOST="$value" ;;
        --rpc-host) RPC_HOST="$value" ;;
        --client-api-url) CLIENT_API_URL="$value" ;;
        --client-rpc-url) CLIENT_RPC_URL="$value" ;;
        --web-port) WEB_PORT="$value" ;;
        --rpc-port) RPC_PORT="$value" ;;
      esac
      shift 2
      ;;
    --skip-pull)
      SKIP_PULL="true"
      shift
      ;;
    --build)
      BUILD_IMAGE="true"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

command -v docker >/dev/null 2>&1 || { echo "Docker is required." >&2; exit 1; }
docker compose version >/dev/null 2>&1 || { echo "Docker Compose v2 is required (docker compose)." >&2; exit 1; }
command -v openssl >/dev/null 2>&1 || { echo "OpenSSL is required to generate the Master secret." >&2; exit 1; }

if [[ -n "$TAG" && ! "$TAG" =~ ^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$ ]]; then
  echo "Invalid Docker tag: $TAG" >&2
  exit 2
fi
if [[ "$BUILD_IMAGE" == "true" && -z "$TAG" ]]; then
  echo "--build requires an explicit --tag." >&2
  exit 2
fi

validate_port() {
  local name="$1"
  local port="$2"
  if [[ ! "$port" =~ ^[0-9]{1,5}$ ]] || ((10#$port < 1 || 10#$port > 65535)); then
    echo "$name must be an integer from 1 to 65535." >&2
    exit 2
  fi
}

validate_port "--web-port" "$WEB_PORT"
validate_port "--rpc-port" "$RPC_PORT"

mkdir -p "$DEPLOY_DIR/data"
DEPLOY_DIR="$(cd -- "$DEPLOY_DIR" && pwd)"
ENV_FILE="$DEPLOY_DIR/.env"
COMPOSE_FILE="$DEPLOY_DIR/docker-compose.yml"
chmod 700 "$DEPLOY_DIR" "$DEPLOY_DIR/data"
cp "$COMPOSE_SOURCE" "$COMPOSE_FILE"

if [[ ! -f "$ENV_FILE" ]]; then
  if [[ -z "$API_HOST" && -t 0 ]]; then
    read -r -p "Master public host (domain or IP): " API_HOST
  fi
  if [[ -z "$API_HOST" ]]; then
    echo "Set --api-host when initializing a new deployment." >&2
    exit 2
  fi
  if [[ "$API_HOST" == *://* || "$API_HOST" == */* || "$API_HOST" == *[[:space:]]* ]]; then
    echo "--api-host must be a hostname or IP without scheme, port, or path." >&2
    exit 2
  fi

  RPC_HOST="${RPC_HOST:-$API_HOST}"
  if [[ "$RPC_HOST" == *://* || "$RPC_HOST" == */* || "$RPC_HOST" == *[[:space:]]* ]]; then
    echo "--rpc-host must be a hostname or IP without scheme, port, or path." >&2
    exit 2
  fi

  API_URL_HOST="$API_HOST"
  RPC_URL_HOST="$RPC_HOST"
  if [[ "$API_HOST" == *:* && "$API_HOST" != \[*\] ]]; then API_URL_HOST="[$API_HOST]"; fi
  if [[ "$RPC_HOST" == *:* && "$RPC_HOST" != \[*\] ]]; then RPC_URL_HOST="[$RPC_HOST]"; fi

  CLIENT_API_URL="${CLIENT_API_URL:-http://${API_URL_HOST}:${WEB_PORT}}"
  CLIENT_RPC_URL="${CLIENT_RPC_URL:-grpc://${RPC_URL_HOST}:${RPC_PORT}}"
  case "$CLIENT_API_URL" in
    http://*) API_SCHEME="http" ;;
    https://*) API_SCHEME="https" ;;
    *) echo "--client-api-url must start with http:// or https://." >&2; exit 2 ;;
  esac
  case "$CLIENT_RPC_URL" in
    grpc://*|ws://*|wss://*) ;;
    *) echo "--client-rpc-url must start with grpc://, ws://, or wss://." >&2; exit 2 ;;
  esac

  IMAGE="vaalacat/frp-panel:${TAG:-alpha}"
  APP_GLOBAL_SECRET="$(openssl rand -hex 32)"
  umask 077
  {
    printf 'COMPOSE_PROJECT_NAME=frp-panel\n'
    printf 'FRP_PANEL_CONTAINER_NAME=frp-panel-master\n'
    printf 'FRP_PANEL_IMAGE=%s\n' "$IMAGE"
    printf 'MASTER_WEB_PORT=%s\n' "$WEB_PORT"
    printf 'MASTER_RPC_PORT=%s\n' "$RPC_PORT"
    printf 'APP_GLOBAL_SECRET=%s\n' "$APP_GLOBAL_SECRET"
    printf 'MASTER_API_HOST=%s\n' "$API_HOST"
    printf 'MASTER_RPC_HOST=%s\n' "$RPC_HOST"
    printf 'MASTER_API_SCHEME=%s\n' "$API_SCHEME"
    printf 'CLIENT_API_URL=%s\n' "$CLIENT_API_URL"
    printf 'CLIENT_RPC_URL=%s\n' "$CLIENT_RPC_URL"
  } > "$ENV_FILE"
  chmod 600 "$ENV_FILE"
  unset APP_GLOBAL_SECRET
else
  chmod 600 "$ENV_FILE"
fi

cd "$DEPLOY_DIR"
if [[ -n "$TAG" ]]; then
  export FRP_PANEL_IMAGE="vaalacat/frp-panel:${TAG}"
fi

if [[ "$BUILD_IMAGE" == "true" ]]; then
  "$ROOT_DIR/scripts/build-image.sh" --tag "$TAG" --load
  SKIP_PULL="true"
fi

if [[ "$SKIP_PULL" != "true" ]]; then
  docker compose -f "$COMPOSE_FILE" pull master
fi
docker compose -f "$COMPOSE_FILE" up -d --remove-orphans

container_id="$(docker compose -f "$COMPOSE_FILE" ps -q master)"
if [[ -z "$container_id" ]]; then
  echo "Master container was not created." >&2
  docker compose -f "$COMPOSE_FILE" logs --tail=80 master >&2 || true
  exit 1
fi

healthy="false"
for _ in {1..30}; do
  health="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$container_id" 2>/dev/null || true)"
  if [[ "$health" == "healthy" ]]; then
    healthy="true"
    break
  fi
  if [[ "$health" == "unhealthy" || "$health" == "exited" ]]; then
    break
  fi
  sleep 2
done

if [[ "$healthy" != "true" ]]; then
  echo "Master did not become healthy. Recent logs:" >&2
  docker compose -f "$COMPOSE_FILE" logs --tail=80 master >&2 || true
  exit 1
fi

if [[ -n "$TAG" ]]; then
  env_tmp="$(mktemp "$DEPLOY_DIR/.env.XXXXXX")"
  sed '/^FRP_PANEL_IMAGE=/d' "$ENV_FILE" > "$env_tmp"
  printf 'FRP_PANEL_IMAGE=vaalacat/frp-panel:%s\n' "$TAG" >> "$env_tmp"
  chmod 600 "$env_tmp"
  mv "$env_tmp" "$ENV_FILE"
fi

printf 'Master is healthy. Data directory: %s/data\n' "$DEPLOY_DIR"
printf 'Web UI: %s\n' "$(sed -n 's/^CLIENT_API_URL=//p' "$ENV_FILE" | head -n 1)"
printf 'First registered user becomes the administrator. The generated secret remains in %s (mode 600).\n' "$ENV_FILE"
