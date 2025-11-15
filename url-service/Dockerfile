FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git wget && \
    GRPC_HEALTH_PROBE_VERSION=v0.4.14 && \
    wget -qO /bin/grpc_health_probe https://github.com/grpc-ecosystem/grpc-health-probe/releases/download/${GRPC_HEALTH_PROBE_VERSION}/grpc_health_probe-linux-amd64 && \
    chmod +x /bin/grpc_health_probe

WORKDIR /app

COPY . .

RUN go mod download
RUN go build -o url-service main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/url-service .
COPY --from=builder /bin/grpc_health_probe /bin/grpc_health_probe
EXPOSE 50051
CMD ["./url-service"]
