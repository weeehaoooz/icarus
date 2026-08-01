# Build Stage
FROM golang:1.26.3-alpine AS builder
WORKDIR /app

# Copy and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code and build statically compiled binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o auth-server cmd/auth-server/main.go

# Production Stage
FROM alpine:3.19
WORKDIR /app

# Create required directories for keys and SQLite database
RUN mkdir -p /app/keys /app/data

# Copy the compiled binary
COPY --from=builder /app/auth-server .

# Expose the service port
EXPOSE 8080

# Environment defaults
ENV PORT=8080
ENV KEYS_DIR=/app/keys
ENV DATABASE_PATH=/app/data/auth.db

# Run the server
CMD ["./auth-server"]
