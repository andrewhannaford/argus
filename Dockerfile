FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w" -o /out/argus-server ./cmd/server

# Digest-pinned base; server is a static binary and runs non-root.
FROM alpine:3.24.1@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b
RUN addgroup -S argus && adduser -S -G argus -u 10001 argus
COPY --from=build /out/argus-server /opt/server
USER argus
EXPOSE 8080
ENTRYPOINT ["/opt/server"]
