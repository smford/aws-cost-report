# Build stage
FROM golang:1.27-alpine@sha256:738d1cf061836894ff6bb8c33881080ac66de8cf0586615012a0c8f592649cfa AS builder

WORKDIR /src

# Download dependencies first for caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build statically linked binary
ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -ldflags="-s -w" -o /bin/aws-cost-report ./cmd/aws-cost-report

# Runtime stage
FROM alpine:3.24@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6

# Install ca-certificates (HTTPS/AWS API calls) and tzdata (timezones)
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user and configuration directory
RUN addgroup -S aws-cost-report && adduser -S aws-cost-report -G aws-cost-report && \
    mkdir -p /etc/aws-cost-report && \
    chown -R aws-cost-report:aws-cost-report /etc/aws-cost-report

COPY --from=builder /bin/aws-cost-report /usr/local/bin/aws-cost-report

USER aws-cost-report
WORKDIR /etc/aws-cost-report

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget -q -O- http://127.0.0.1:8080/healthz || exit 1

ENTRYPOINT ["/usr/local/bin/aws-cost-report"]
CMD ["-config", "/etc/aws-cost-report/aws-cost-report.yaml"]
