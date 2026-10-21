#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE_SOURCE="$ROOT_DIR/deploy/master/docker-compose.yml"
DEPLOY_DIR="${FRP_PANEL_DEPLOY_DIR:-/opt/frp-panel}"
TAG=""
AUTO_TAG="true"
BUILD_IMAGE="true"
IMAGE_REPOSITORY="${FRP_WEB_PANEL_IMAGE_REPOSITORY:-lujuya/frp-web-panel}"
API_HOST="${FRP_PANEL_API_HOST:-${FRP_PANEL_DOMAIN:-}}"
RPC_HOST="${FRP_PANEL_RPC_HOST:-}"
CLIENT_API_URL="${FRP_PANEL_CLIENT_API_URL:-}"
CLIENT_RPC_URL="${FRP_PANEL_CLIENT_RPC_URL:-}"
WEB_PORT="${FRP_PANEL_WEB_PORT:-9000}"
RPC_PORT="${FRP_PANEL_RPC_PORT:-9001}"
SKIP_PULL="false"
API_HOST_SET="false"
RPC_HOST_SET="false"
CLIENT_API_URL_SET="false"
CLIENT_RPC_URL_SET="false"
WEB_PORT_SET="false"
RPC_PORT_SET="false"

[[ -n "$API_HOST" ]] && API_HOST_SET="true"
[[ -n "$RPC_HOST" ]] && RPC_HOST_SET="true"
[[ -n "$CLIENT_API_URL" ]] && CLIENT_API_URL_SET="true"
[[ -n "$CLIENT_RPC_URL" ]] && CLIENT_RPC_URL_SET="true"
[[ -n "${FRP_PANEL_WEB_PORT:-}" ]] && WEB_PORT_SET="true"
[[ -n "${FRP_PANEL_RPC_PORT:-}" ]] && RPC_PORT_SET="true"

usage() {
  cat <<'EOF'
Initialize or update the FRP Web Panel Master Docker Compose deployment.

Usage:
  sudo scripts/deploy-master.sh

Options:
  --dir <path>             Deployment directory (default: /opt/frp-panel)
  --tag <tag>              Override the default local-<commit> image tag
  --no-build               Use an existing image; requires --tag <tag>
  --build                  Build and load the image locally (default; kept as an explicit alias)
  --skip-pull              Skip pulling when the selected image is already loaded locally
  --domain <domain>        Optional domain for the Master API and RPC
  --api-host <host>        Public IP or domain for the Master API (default: auto-detected IP)
  --rpc-host <host>        Public host used for Master RPC (default: API host)
  --client-api-url <url>   URL nodes use for the API (default: http://<api-host>:<web-port>)
  --client-rpc-url <url>   URL nodes use for RPC (default: grpc://<rpc-host>:<rpc-port>)
  --web-port <port>        Host port published for the web UI (default: 9000)
  --rpc-port <port>        Host port published for RPC (default: 9001)
  -h, --help               Show this help

For a reverse proxy, pass its externally reachable URLs, for example:
  --client-api-url https://panel.example.com
  --client-rpc-url wss://panel.example.com

Environment overrides (useful for non-interactive deployment):
  FRP_PANEL_DOMAIN=panel.example.com
  FRP_PANEL_API_HOST=192.0.2.10
  FRP_PANEL_RPC_HOST=panel.example.com
EOF
}

while (($#)); do
  case "$1" in
    --dir|--tag|--domain|--api-host|--rpc-host|--client-api-url|--client-rpc-url|--web-port|--rpc-port)
      [[ $# -ge 2 ]] || { echo "Missing value for $1" >&2; exit 2; }
      key="$1"
      value="$2"
      case "$key" in
        --dir) DEPLOY_DIR="$value" ;;
        --tag) TAG="$value"; AUTO_TAG="false" ;;
        --domain|--api-host) API_HOST="$value"; API_HOST_SET="true" ;;
        --rpc-host) RPC_HOST="$value"; RPC_HOST_SET="true" ;;
        --client-api-url) CLIENT_API_URL="$value"; CLIENT_API_URL_SET="true" ;;
        --client-rpc-url) CLIENT_RPC_URL="$value"; CLIENT_RPC_URL_SET="true" ;;
        --web-port) WEB_PORT="$value"; WEB_PORT_SET="true" ;;
        --rpc-port) RPC_PORT="$value"; RPC_PORT_SET="true" ;;
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
    --no-build)
      BUILD_IMAGE="false"
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
command -v git >/dev/null 2>&1 || { echo "Git is required to identify the source commit." >&2; exit 1; }

SOURCE_COMMIT="$(git -C "$ROOT_DIR" rev-parse HEAD 2>/dev/null)" || {
  echo "Run this script from a Git checkout of the project." >&2
  exit 1
}
if [[ -n "$(git -C "$ROOT_DIR" status --porcelain 2>/dev/null)" ]]; then
  echo "The source tree has uncommitted changes. Commit them before deploying." >&2
  exit 2
fi
if [[ -z "$TAG" ]]; then
  TAG="local-${SOURCE_COMMIT:0:12}"
fi
if [[ ! "$TAG" =~ ^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$ ]]; then
  echo "Invalid Docker tag: $TAG" >&2
  exit 2
fi
if [[ "$BUILD_IMAGE" == "false" && "$AUTO_TAG" == "true" ]]; then
  echo "--no-build requires --tag <tag>." >&2
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

validate_host() {
  local name="$1"
  local host="$2"
  local value="$2"
  local octet
  local -a octets labels

  if [[ -z "$host" || "$host" == *://* || "$host" == */* || "$host" == *[[:space:]]* ]]; then
    echo "$name must be a hostname or IP without scheme, port, or path." >&2
    exit 2
  fi

  if [[ "$host" == \[*\] ]]; then
    value="${host:1:${#host}-2}"
  fi
  if [[ "$value" == *:* ]]; then
    if [[ ! "$value" =~ ^[A-Fa-f0-9:.%]+$ ]]; then
      echo "$name must be a valid IPv6 address without a port." >&2
      exit 2
    fi
    return
  fi

  if [[ "$value" =~ ^[0-9]+(\.[0-9]+){3}$ ]]; then
    IFS='.' read -r -a octets <<< "$value"
    for octet in "${octets[@]}"; do
      if ((10#$octet > 255)); then
        echo "$name contains an invalid IPv4 address." >&2
        exit 2
      fi
    done
    return
  fi

  if (( ${#value} > 253 )) || [[ "$value" == .* || "$value" == *. || "$value" == *..* ]] || [[ ! "$value" =~ ^[A-Za-z0-9.-]+$ ]]; then
    echo "$name must be a hostname or IP without scheme, port, or path." >&2
    exit 2
  fi
  IFS='.' read -r -a labels <<< "$value"
  for label in "${labels[@]}"; do
    if (( ${#label} > 63 )) || [[ ! "$label" =~ ^[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?$ ]]; then
      echo "$name contains an invalid hostname label." >&2
      exit 2
    fi
  done
}

detect_default_host() {
  local detected=""
  if command -v ip >/dev/null 2>&1; then
    detected="$(ip -4 route get 1.1.1.1 2>/dev/null | awk '{for (i = 1; i <= NF; i++) if ($i == "src" && i < NF) {print $(i + 1); exit}}')"
  fi
  if [[ -z "$detected" ]] && command -v hostname >/dev/null 2>&1; then
    detected="$(hostname -I 2>/dev/null | tr ' ' '\n' | awk '$1 ~ /^[0-9]+\./ && $1 !~ /^127\./ && $1 !~ /^169\.254\./ {print; exit}')"
  fi
  printf '%s' "$detected"
}

read_env_value() {
  local key="$1"
  [[ -f "$ENV_FILE" ]] || return 0
  awk -v key="$key" 'index($0, key "=") == 1 {sub(/^[^=]*=/, "", $0); print; exit}' "$ENV_FILE"
}

format_url_host() {
  local host="$1"
  if [[ "$host" == *:* && "$host" != \[*\] ]]; then
    printf '[%s]' "$host"
  else
    printf '%s' "$host"
  fi
}

set_env_value() {
  local key="$1"
  local value="$2"
  local env_tmp
  env_tmp="$(mktemp "$ENV_FILE.XXXXXX")"
  awk -v key="$key" -v value="$value" '
    BEGIN { updated = 0 }
    index($0, key "=") == 1 { print key "=" value; updated = 1; next }
    { print }
    END { if (!updated) print key "=" value }
  ' "$ENV_FILE" > "$env_tmp"
  chmod 600 "$env_tmp"
  mv "$env_tmp" "$ENV_FILE"
}

mkdir -p "$DEPLOY_DIR/data"
DEPLOY_DIR="$(cd -- "$DEPLOY_DIR" && pwd)"
ENV_FILE="$DEPLOY_DIR/.env"
COMPOSE_FILE="$DEPLOY_DIR/docker-compose.yml"
chmod 700 "$DEPLOY_DIR" "$DEPLOY_DIR/data"
cp "$COMPOSE_SOURCE" "$COMPOSE_FILE"

OLD_API_HOST=""
OLD_RPC_HOST=""
OLD_CLIENT_API_URL=""
OLD_CLIENT_RPC_URL=""
OLD_WEB_PORT=""
OLD_RPC_PORT=""
if [[ -f "$ENV_FILE" ]]; then
  OLD_API_HOST="$(read_env_value MASTER_API_HOST)"
  OLD_RPC_HOST="$(read_env_value MASTER_RPC_HOST)"
  OLD_CLIENT_API_URL="$(read_env_value CLIENT_API_URL)"
  OLD_CLIENT_RPC_URL="$(read_env_value CLIENT_RPC_URL)"
  OLD_WEB_PORT="$(read_env_value MASTER_WEB_PORT)"
  OLD_RPC_PORT="$(read_env_value MASTER_RPC_PORT)"
  [[ "$API_HOST_SET" == "true" ]] || API_HOST="$OLD_API_HOST"
  [[ "$RPC_HOST_SET" == "true" ]] || RPC_HOST="$OLD_RPC_HOST"
  [[ "$CLIENT_API_URL_SET" == "true" ]] || CLIENT_API_URL="$OLD_CLIENT_API_URL"
  [[ "$CLIENT_RPC_URL_SET" == "true" ]] || CLIENT_RPC_URL="$OLD_CLIENT_RPC_URL"
  [[ "$WEB_PORT_SET" == "true" ]] || WEB_PORT="$OLD_WEB_PORT"
  [[ "$RPC_PORT_SET" == "true" ]] || RPC_PORT="$OLD_RPC_PORT"
fi

WEB_PORT="${WEB_PORT:-9000}"
RPC_PORT="${RPC_PORT:-9001}"
if [[ -z "$API_HOST" ]]; then
  API_HOST="$(detect_default_host)"
  if [[ -z "$API_HOST" ]]; then
    echo "Cannot detect a local IP address. Set --domain/--api-host or FRP_PANEL_DOMAIN/FRP_PANEL_API_HOST." >&2
    exit 2
  fi
  echo "Using detected Master host: $API_HOST"
fi
if [[ "$RPC_HOST_SET" != "true" ]]; then
  if [[ -n "$OLD_RPC_HOST" && -n "$OLD_API_HOST" && "$OLD_RPC_HOST" != "$OLD_API_HOST" ]]; then
    RPC_HOST="$OLD_RPC_HOST"
  else
    RPC_HOST="$API_HOST"
  fi
fi
RPC_HOST="${RPC_HOST:-$API_HOST}"
validate_host "--api-host" "$API_HOST"
validate_host "--rpc-host" "$RPC_HOST"
validate_port "--web-port" "$WEB_PORT"
validate_port "--rpc-port" "$RPC_PORT"

API_URL_HOST="$(format_url_host "$API_HOST")"
RPC_URL_HOST="$(format_url_host "$RPC_HOST")"
DEFAULT_CLIENT_API_URL="http://${API_URL_HOST}:${WEB_PORT}"
DEFAULT_CLIENT_RPC_URL="grpc://${RPC_URL_HOST}:${RPC_PORT}"

if [[ "$CLIENT_API_URL_SET" != "true" ]]; then
  OLD_DEFAULT_CLIENT_API_URL=""
  if [[ -n "$OLD_API_HOST" && -n "$OLD_WEB_PORT" ]]; then
    OLD_DEFAULT_CLIENT_API_URL="http://$(format_url_host "$OLD_API_HOST"):${OLD_WEB_PORT}"
  fi
  if [[ -z "$CLIENT_API_URL" || "$CLIENT_API_URL" == "$OLD_DEFAULT_CLIENT_API_URL" ]]; then
    CLIENT_API_URL="$DEFAULT_CLIENT_API_URL"
  fi
fi
if [[ "$CLIENT_RPC_URL_SET" != "true" ]]; then
  OLD_DEFAULT_CLIENT_RPC_URL=""
  if [[ -n "$OLD_RPC_HOST" && -n "$OLD_RPC_PORT" ]]; then
    OLD_DEFAULT_CLIENT_RPC_URL="grpc://$(format_url_host "$OLD_RPC_HOST"):${OLD_RPC_PORT}"
  fi
  if [[ -z "$CLIENT_RPC_URL" || "$CLIENT_RPC_URL" == "$OLD_DEFAULT_CLIENT_RPC_URL" ]]; then
    CLIENT_RPC_URL="$DEFAULT_CLIENT_RPC_URL"
  fi
fi
case "$CLIENT_API_URL" in
  http://*) API_SCHEME="http" ;;
  https://*) API_SCHEME="https" ;;
  *) echo "--client-api-url must start with http:// or https://." >&2; exit 2 ;;
esac
case "$CLIENT_RPC_URL" in
  grpc://*|ws://*|wss://*) ;;
  *) echo "--client-rpc-url must start with grpc://, ws://, or wss://." >&2; exit 2 ;;
esac

IMAGE="$IMAGE_REPOSITORY:$TAG"

if [[ ! -f "$ENV_FILE" ]]; then
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
else
  chmod 600 "$ENV_FILE"
  if [[ -z "$(read_env_value APP_GLOBAL_SECRET)" ]]; then
    set_env_value APP_GLOBAL_SECRET "$(openssl rand -hex 32)"
  fi
  set_env_value FRP_PANEL_IMAGE "$IMAGE"
  set_env_value MASTER_WEB_PORT "$WEB_PORT"
  set_env_value MASTER_RPC_PORT "$RPC_PORT"
  set_env_value MASTER_API_HOST "$API_HOST"
  set_env_value MASTER_RPC_HOST "$RPC_HOST"
  set_env_value MASTER_API_SCHEME "$API_SCHEME"
  set_env_value CLIENT_API_URL "$CLIENT_API_URL"
  set_env_value CLIENT_RPC_URL "$CLIENT_RPC_URL"
fi
chmod 600 "$ENV_FILE"
unset APP_GLOBAL_SECRET

cd "$DEPLOY_DIR"

if [[ "$BUILD_IMAGE" == "true" ]]; then
  "$ROOT_DIR/scripts/build-image.sh" --tag "$TAG" --load
  SKIP_PULL="true"
fi

docker compose -f "$COMPOSE_FILE" config --quiet
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

printf 'Master is healthy. Data directory: %s/data\n' "$DEPLOY_DIR"
printf 'Source commit: %s\n' "$SOURCE_COMMIT"
printf 'Image tag: %s\n' "$TAG"
# Generated node Compose files reference the floating `latest` tag. A local
# build loads that tag too, so the node commands below stay compatible with
# the Compose files produced by the Master.
if [[ "$BUILD_IMAGE" == "true" ]]; then
  printf 'Node image tag: %s:latest\n' "$IMAGE_REPOSITORY"
else
  printf 'Node image tag: %s:latest (pull it, or build locally to load this tag)\n' "$IMAGE_REPOSITORY"
fi
printf 'Node source checkout and build commands:\n'
printf 'cd "$HOME"\n'
printf 'if [ -d frp-web-panel/.git ]; then git -C frp-web-panel fetch origin main; else git clone --branch main https://github.com/yunj-yun/frp-web-panel.git; fi\n'
printf 'git -C frp-web-panel checkout %s\n' "$SOURCE_COMMIT"
printf 'cd frp-web-panel\n'
if [[ "$IMAGE_REPOSITORY" != "lujuya/frp-web-panel" ]]; then
  if [[ "$AUTO_TAG" == "true" ]]; then
    printf 'sudo env FRP_WEB_PANEL_IMAGE_REPOSITORY=%q ./scripts/build-image.sh\n' "$IMAGE_REPOSITORY"
  else
    printf 'sudo env FRP_WEB_PANEL_IMAGE_REPOSITORY=%q ./scripts/build-image.sh --tag %q\n' "$IMAGE_REPOSITORY" "$TAG"
  fi
elif [[ "$AUTO_TAG" == "true" ]]; then
  printf 'sudo ./scripts/build-image.sh\n'
else
  printf 'sudo ./scripts/build-image.sh --tag %q\n' "$TAG"
fi
printf 'Master host: %s\n' "$API_HOST"
printf 'Web UI: %s\n' "$(sed -n 's/^CLIENT_API_URL=//p' "$ENV_FILE" | head -n 1)"
printf 'Node RPC URL: %s\n' "$(sed -n 's/^CLIENT_RPC_URL=//p' "$ENV_FILE" | head -n 1)"
printf 'First registered user becomes the administrator. The generated secret remains in %s (mode 600).\n' "$ENV_FILE"
