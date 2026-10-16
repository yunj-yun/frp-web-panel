#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
IMAGE_REPOSITORY="vaalacat/frp-panel"
TAG=""
PLATFORMS=""
OUTPUT="load"

usage() {
  cat <<'EOF'
Build the unified FRP-Panel image used by Master, Server, and Client.

Usage:
  scripts/build-image.sh --tag <tag> [--platform <platforms>] [--load|--push]

Options:
  --tag <tag>          Docker tag and application version (required)
  --platform <list>    Buildx platform list, e.g. linux/amd64,linux/arm64
  --load               Load a single-platform image into the local Docker engine (default)
  --push               Push the image to vaalacat/frp-panel
  -h, --help           Show this help

Examples:
  scripts/build-image.sh --tag v1.2.3 --load
  scripts/build-image.sh --tag v1.2.3 --platform linux/amd64,linux/arm64 --push
EOF
}

while (($#)); do
  case "$1" in
    --tag)
      [[ $# -ge 2 ]] || { echo "Missing value for --tag" >&2; exit 2; }
      TAG="$2"
      shift 2
      ;;
    --platform)
      [[ $# -ge 2 ]] || { echo "Missing value for --platform" >&2; exit 2; }
      PLATFORMS="$2"
      shift 2
      ;;
    --load)
      OUTPUT="load"
      shift
      ;;
    --push)
      OUTPUT="push"
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

if [[ ! "$TAG" =~ ^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$ ]]; then
  echo "Pass a valid Docker tag with --tag (for example: v1.2.3)." >&2
  exit 2
fi

if [[ "$OUTPUT" == "load" && "$PLATFORMS" == *,* ]]; then
  echo "--load supports one platform; use --push for a multi-platform image." >&2
  exit 2
fi

command -v docker >/dev/null 2>&1 || { echo "Docker is required." >&2; exit 1; }
docker buildx version >/dev/null 2>&1 || { echo "Docker Buildx is required." >&2; exit 1; }

GIT_COMMIT="$(git -C "$ROOT_DIR" rev-parse --short=12 HEAD 2>/dev/null || printf 'unknown')"
BUILD_DATE="$(date -u +'%Y-%m-%dT%H:%M:%SZ')"
IMAGE="${IMAGE_REPOSITORY}:${TAG}"

BUILD_ARGS=(
  docker buildx build
  --file "$ROOT_DIR/Dockerfile.standalone"
  --tag "$IMAGE"
  --build-arg "VERSION=$TAG"
  --build-arg "GIT_COMMIT=$GIT_COMMIT"
  --build-arg "BUILD_DATE=$BUILD_DATE"
)

if [[ -n "$PLATFORMS" ]]; then
  BUILD_ARGS+=(--platform "$PLATFORMS")
fi

if [[ "$OUTPUT" == "push" ]]; then
  BUILD_ARGS+=(--push)
else
  BUILD_ARGS+=(--load)
fi

BUILD_ARGS+=("$ROOT_DIR")
printf 'Building %s (%s)\n' "$IMAGE" "${PLATFORMS:-native platform}"
"${BUILD_ARGS[@]}"
printf 'Image ready: %s\n' "$IMAGE"
