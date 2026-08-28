FROM golang:1.26.7-trixie AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/rakda ./cmd/main
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go install github.com/pressly/goose/v3/cmd/goose@v3.27.3 \
    && cp /go/bin/goose /out/goose

FROM debian:trixie-slim AS migrate
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/goose /usr/local/bin/goose
COPY migrations /migrations
ENV GOOSE_MIGRATION_DIR=/migrations
USER 10001
ENTRYPOINT ["goose"]
CMD ["status"]

FROM debian:trixie-slim AS runtime
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        poppler-utils tesseract-ocr tesseract-ocr-eng tesseract-ocr-ind \
        ca-certificates fontconfig fonts-liberation curl \
    && rm -rf /var/lib/apt/lists/*
RUN useradd --system --uid 10001 --create-home rakda
USER rakda
# configs/.env is deliberately absent: the loader tolerates a missing file and
# real env vars win, so all configuration comes from compose.
WORKDIR /app
COPY --from=build /out/rakda /app/rakda
EXPOSE 8080
ENTRYPOINT ["/app/rakda"]
