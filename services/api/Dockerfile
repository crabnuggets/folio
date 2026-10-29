# Build from repository root: docker build -f services/api/Dockerfile .
FROM node:22.22.2-bookworm-slim AS web-build
WORKDIR /src/apps/web
COPY apps/web/package*.json ./
RUN npm ci
COPY apps/web/ ./
RUN npm run build

FROM golang:1.26.10-bookworm AS build
WORKDIR /src
COPY services/api/go.mod services/api/go.sum ./
RUN --mount=type=secret,id=proxy_ca \
    if [ -f /run/secrets/proxy_ca ]; then SSL_CERT_FILE=/run/secrets/proxy_ca go mod download; else go mod download; fi
COPY services/api/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /folio ./cmd/folio
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /folio /folio
COPY --from=web-build /src/apps/web/dist /app/web/dist
ENV WEB_DIST_DIR=/app/web/dist
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/folio"]
