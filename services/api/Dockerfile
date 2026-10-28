# Build from repository root: docker build -f services/api/Dockerfile .
FROM golang:1.26.10-bookworm AS build
WORKDIR /src
COPY services/api/go.mod services/api/go.sum ./
RUN --mount=type=secret,id=proxy_ca \
    if [ -f /run/secrets/proxy_ca ]; then SSL_CERT_FILE=/run/secrets/proxy_ca go mod download; else go mod download; fi
COPY services/api/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /folio ./cmd/folio
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /folio /folio
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/folio"]
