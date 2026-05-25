# Build any Go service: docker build --build-arg SERVICE=dataset-replay-service -f Dockerfile.go .
FROM golang:1.24-alpine AS builder
ARG SERVICE
WORKDIR /app
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download 2>/dev/null || true
COPY . .
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -o /service ./services/${SERVICE}

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /service /app/service
ENTRYPOINT ["/app/service"]
