# Build stage
FROM golang:1.25.11-alpine3.23 AS builder

ARG VERSION=dev
ARG COMMIT=none

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
    -o /talos ./cmd/talos

# Runtime stage
FROM alpine:3.23

RUN apk add --no-cache ca-certificates docker-cli

COPY --from=builder /talos /usr/local/bin/talos

EXPOSE 3000

VOLUME ["/data"]

ENTRYPOINT ["talos"]
