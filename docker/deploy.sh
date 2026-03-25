#!/bin/bash

set -e

run_compose() { docker compose -p prod -f docker-compose.yaml -f docker/docker-compose.prod.yaml "$@"; }

# build containers with build sections in compose
run_compose build -q

# start project
run_compose up -d --quiet-pull

# show containers status
run_compose ps

# Clear old backend images
docker image prune -f
