#!/bin/bash

set -euo pipefail

compose_files=(-f docker-compose.yaml -f docker/docker-compose.prod.yaml)

# Pull the already-built images published by the CD workflow.
docker-compose -p prod "${compose_files[@]}" pull

# Recreate the production stack from the pulled images and prune removed services.
docker-compose -p prod "${compose_files[@]}" up -d --remove-orphans

# Show the resulting container state for quick deployment diagnostics.
docker-compose -p prod "${compose_files[@]}" ps

# Clear dangling images after the successful rollout.
docker image prune -f
