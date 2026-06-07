# syntax=docker/dockerfile:1.7

ARG GO_VERSION=1.25.6

FROM golang:${GO_VERSION}-alpine AS build

WORKDIR /src

COPY backend/go.mod backend/go.sum ./backend/
WORKDIR /src/backend
RUN go mod download

WORKDIR /src
COPY backend ./backend
COPY schemas ./schemas

WORKDIR /src/backend
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/novelscript-api ./cmd/api

FROM alpine:3.21

RUN addgroup -S app && adduser -S app -G app

WORKDIR /app

COPY --from=build /out/novelscript-api ./novelscript-api
COPY --from=build /src/schemas ./schemas

RUN mkdir -p /app/data/exports && chown -R app:app /app

ENV APP_ENV=production \
    PORT=8080 \
    API_BASE_PATH=/api/v1 \
    PUBLIC_BASE_URL=http://localhost \
    REQUEST_BODY_LIMIT_MB=20 \
    DATA_DIR=/app/data \
    EXPORT_DIR=/app/data/exports \
    SCRIPT_SCHEMA_PATH=/app/schemas/script.schema.json

EXPOSE 8080

USER app

ENTRYPOINT ["./novelscript-api"]
