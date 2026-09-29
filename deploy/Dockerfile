# syntax=docker/dockerfile:1.7
# Build from the repository root:  docker build -f deploy/Dockerfile -t gcplab .
#
# Targets:
#   runtime (default, last stage) - API + lab plane + grader, simulator (F0) and emulators (F1)
#   runtime-gcloud                - same binaries plus the Google Cloud CLI for real sandboxes (F2_DRIVER=gcloud)

FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM golang:1.25 AS go
WORKDIR /src
ENV CGO_ENABLED=0 GOTOOLCHAIN=local
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
RUN go build -trimpath -ldflags="-s -w" -o /out/gcplab ./cmd/gcplab \
 && go build -trimpath -ldflags="-s -w" -o /out/grader-worker ./cmd/grader-worker \
 && go build -trimpath -ldflags="-s -w" -o /out/labctl ./cmd/labctl
COPY content/ content/
# content-as-code gate: an image is never built with broken labs
RUN /out/labctl validate

FROM gcr.io/google.com/cloudsdktool/google-cloud-cli:slim AS runtime-gcloud
RUN apt-get update && apt-get install -y --no-install-recommends kubectl && rm -rf /var/lib/apt/lists/* \
 && useradd --system --uid 65532 --home /app app
WORKDIR /app
COPY --from=go /out/ /usr/local/bin/
COPY --from=go /src/content /app/content
COPY --from=web /src/web/out /app/web
ENV CONTENT_DIR=/app/content WEB_DIR=/app/web DATA_FILE=/tmp/gcplab.json F2_DRIVER=gcloud
EXPOSE 8080
USER 65532
ENTRYPOINT ["/usr/local/bin/gcplab"]

FROM gcr.io/distroless/static-debian12:nonroot AS runtime
WORKDIR /app
COPY --from=go /out/ /usr/local/bin/
COPY --from=go /src/content /app/content
COPY --from=web /src/web/out /app/web
ENV CONTENT_DIR=/app/content WEB_DIR=/app/web DATA_FILE=/tmp/gcplab.json
EXPOSE 8080
USER nonroot
ENTRYPOINT ["/usr/local/bin/gcplab"]
