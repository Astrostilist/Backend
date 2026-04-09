#!/bin/bash

set -euo pipefail

project_name="${COMPOSE_PROJECT_NAME:-prod}"
compose_files=(-f docker-compose.yaml -f docker/docker-compose.prod.yaml)

# Pull the already-built images published by the CD workflow.
docker compose --project-name "$project_name" "${compose_files[@]}" pull

# Recreate the production stack from the pulled images and prune removed services.
docker compose --project-name "$project_name" "${compose_files[@]}" up -d --remove-orphans

# Show the resulting container state for quick deployment diagnostics.
docker compose --project-name "$project_name" "${compose_files[@]}" ps

# Clear dangling images after the successful rollout.
docker image prune -f
