#!/usr/bin/env sh
# Pull-based deploy of one self-hosted stack; launchd runs it every minute.
# Usage: deploy/deploy.sh <env-file>. The env file sets DEPLOY_BRANCH, PUBLIC_URL and REGISTRY.
# Nothing from GitHub runs here: we fetch the branch tip and pull the images CI built for it.
set -eu

conf() { sed -n "s/^$1=//p" "$ENV_FILE" | tail -n 1; }
log() { echo "$(date '+%F %T') $*"; }
notify() { osascript -e "display notification \"$1\" with title \"buddy deploy\"" 2>/dev/null || true; }
stack() { "$REPO/deploy/stack.sh" "$ENV_FILE" "$@"; }
start() { stack up -d --wait --wait-timeout 300 --no-build --remove-orphans; }
smoke() {
  CF_ACCESS_CLIENT_ID=$(conf CF_ACCESS_CLIENT_ID) CF_ACCESS_CLIENT_SECRET=$(conf CF_ACCESS_CLIENT_SECRET) \
    "$REPO/deploy/smoke.sh" "$(conf PUBLIC_URL)"
}

# git checkout below rewrites this file; sh reads a whole function before running it.
main() {
  ENV_FILE=$1
  DIR=$(dirname "$ENV_FILE")
  REPO=$(cd "$(dirname "$0")/.." && pwd)
  BRANCH=$(conf DEPLOY_BRANCH)
  REGISTRY=$(conf REGISTRY)

  # One deploy at a time; mkdir is atomic.
  mkdir "$DIR/deploy.lock" 2>/dev/null || exit 0
  trap 'rmdir "$DIR/deploy.lock"' EXIT

  git -C "$REPO" fetch -q origin "$BRANCH"
  SHA=$(git -C "$REPO" rev-parse FETCH_HEAD)
  [ "$SHA" != "$(cat "$DIR/deployed-sha" 2>/dev/null)" ] || exit 0
  [ "$SHA" != "$(cat "$DIR/failed-sha" 2>/dev/null)" ] || exit 0
  # CI pushes images only after the tests pass; until then wait for the next run.
  docker manifest inspect "$REGISTRY/buddy-backend:$SHA" >/dev/null 2>&1 || exit 0
  docker manifest inspect "$REGISTRY/buddy-frontend:$SHA" >/dev/null 2>&1 || exit 0

  log "deploying $BRANCH $SHA"
  PREV=$(git -C "$REPO" rev-parse HEAD)
  if [ -f "$DIR/release.env" ]; then cp "$DIR/release.env" "$DIR/release.env.prev"; else rm -f "$DIR/release.env.prev"; fi
  git -C "$REPO" checkout -q --detach "$SHA"
  printf 'BACKEND_IMAGE=%s\nFRONTEND_IMAGE=%s\n' \
    "$REGISTRY/buddy-backend:$SHA" "$REGISTRY/buddy-frontend:$SHA" > "$DIR/release.env"

  if stack pull -q && start && smoke; then
    echo "$SHA" > "$DIR/deployed-sha"
    log "deployed $SHA"
    notify "$BRANCH: deployed $(printf %.7s "$SHA")"
    exit 0
  fi

  log "deploy of $SHA failed, rolling back to $PREV"
  echo "$SHA" > "$DIR/failed-sha"
  git -C "$REPO" checkout -q --detach "$PREV"
  if [ -f "$DIR/release.env.prev" ]; then mv "$DIR/release.env.prev" "$DIR/release.env"; else rm -f "$DIR/release.env"; fi
  start || log "rollback failed too, fix the stack by hand"
  notify "$BRANCH: deploy of $(printf %.7s "$SHA") failed, rolled back"
  exit 1
}

main "$@"