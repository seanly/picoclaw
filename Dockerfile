# ============================================================
# Stage 1: Build the picoclaw binary
# ============================================================
FROM golang:1.26.0-alpine AS builder

RUN apk add --no-cache git make

WORKDIR /src

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN make build

# ============================================================
# Stage 2: Minimal runtime image
# ============================================================
FROM alpine:3.23

RUN apk add --no-cache ca-certificates tzdata curl busybox-extras bash git vim jq

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -q --spider http://localhost:18790/health || exit 1
# ca-certificates, tzdata, tini (init for zombie reaping when running as PID 1)
RUN apk add --no-cache ca-certificates tzdata && \
    apk add --no-cache tini --repository=http://dl-cdn.alpinelinux.org/alpine/edge/community

# Copy binary
COPY --from=builder /src/build/picoclaw /usr/local/bin/picoclaw

# Create non-root user and group
RUN addgroup -g 1000 picoclaw && \
    adduser -D -u 1000 -G picoclaw picoclaw

# Switch to non-root user
USER picoclaw

# Run onboard to create initial directories and config
RUN /usr/local/bin/picoclaw onboard

# tini as PID 1: reaps zombies, forwards signals to process group (-g)
ENTRYPOINT ["tini", "-g", "--", "picoclaw"]
CMD ["gateway"]
