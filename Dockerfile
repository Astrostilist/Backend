# syntax=docker/dockerfile:1.7

# ---- build stage ----------------------------------------------------
FROM golang:1.25-alpine AS builder

WORKDIR /src

# отдельный шаг с зависимостями — позволяет переиспользовать слой кеша
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

# статический бинарь
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build \
      -trimpath -ldflags "-s -w" \
      -o /out/astro-backend ./cmd

# бинарь для создания супер-пользователя
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build \
      -trimpath -ldflags "-s -w" \
      -o /out/astro-backend-superadmin ./cmd/superadmin

# ---- runtime stage --------------------------------------------------
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S app && adduser -S -G app app

WORKDIR /app

COPY --from=builder /out/astro-backend /app/astro-backend
COPY --from=builder /out/astro-backend-superadmin /app/astro-backend-superadmin
COPY migrations /app/migrations

USER app

EXPOSE 8080

ENTRYPOINT ["/app/astro-backend"]
