#!/bin/bash
#
# deploy.sh — обновление продакшн-стека из docker-compose.yaml.
# Образы подтягиваются из registry (по умолчанию $APP_IMAGE) и
# перекатываются без даунтайма (--remove-orphans удаляет ушедшие сервисы).

set -euo pipefail

project_name="${COMPOSE_PROJECT_NAME:-prod}"
compose_file="docker-compose.yaml"

# Pull готовые образы, опубликованные CD.
docker compose --project-name "$project_name" -f "$compose_file" pull

# Пересобрать стек из свежих образов.
docker compose --project-name "$project_name" -f "$compose_file" up -d --remove-orphans

# Статус для диагностики.
docker compose --project-name "$project_name" -f "$compose_file" ps

# Почистить висячие образы после успешного релиза.
docker image prune -f
