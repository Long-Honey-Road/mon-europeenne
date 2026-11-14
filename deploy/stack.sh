#!/usr/bin/env sh
# Runs docker compose for a self-hosted stack (staging, prod) with the prod override.
# Usage: deploy/stack.sh <env-file> <compose args...>, e.g. deploy/stack.sh ~/srv/buddy-staging/.env up -d --build --wait
set -eu
[ $# -ge 2 ] || { echo "usage: $0 <env-file> <compose args...>"; exit 2; }
ENV_FILE=$1
shift
DIR=$(cd "$(dirname "$0")" && pwd)
# Each stack gets its own containers, network and volumes, so staging never touches the dev stack.
NAME=$(sed -n 's/^COMPOSE_PROJECT_NAME=//p' "$ENV_FILE")
[ -n "$NAME" ] || { echo "set COMPOSE_PROJECT_NAME in $ENV_FILE"; exit 1; }
# The image tags of the current release, written by deploy.sh; manual commands use them too.
# i think its bad, but for now its okay(usually this is the way to the critical issue, i guess)
RELEASE="$(dirname "$ENV_FILE")/release.env"
[ -f "$RELEASE" ] || RELEASE=/dev/null
# Stand rules only where the env file points to a folder with them, never by default.
STAND=$(sed -n 's/^STAND_DIR=//p' "$ENV_FILE")
set -- -f "$DIR/docker-compose.yml" -f "$DIR/compose.prod.yml" ${STAND:+-f "$DIR/compose.stand.yml"} "$@"
exec docker compose -p "$NAME" --env-file "$ENV_FILE" --env-file "$RELEASE" "$@"
