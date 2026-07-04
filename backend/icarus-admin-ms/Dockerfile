# Build Stage
FROM golang:1.26.3-alpine AS builder
WORKDIR /app

# Copy and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code and build statically compiled binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o admin-server cmd/admin-server/main.go

# Production Stage
FROM alpine:3.19
WORKDIR /app

# Create data directory for SQLite database
RUN mkdir -p /app/data

# Copy the compiled binary
COPY --from=builder /app/admin-server .

# Expose the service port
EXPOSE 8081

# Environment defaults
ENV PORT=8081
ENV DATABASE_PATH=/app/data/platform.db
ENV AUTH_MS_CERTS_URL=http://auth-ms:8080/certs?format=pem

# Run the server
CMD ["./admin-server"]
