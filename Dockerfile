FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder

WORKDIR /app

# Install build dependencies if needed
RUN apk add --no-cache git ca-certificates

# Download dependencies first for better layer cache
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build static binary
# -ldflags="-w -s" reduces binary size
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags="-w -s" -o /out/stash-rule .

# Final stage
FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /out/stash-rule /stash-rule

EXPOSE 8080

USER 65532:65532

ENTRYPOINT ["/stash-rule"]
