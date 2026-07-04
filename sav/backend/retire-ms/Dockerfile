# Build Stage
FROM golang:1.26.3-alpine AS builder
WORKDIR /app

# Copy and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code and build statically compiled binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o retire-server cmd/retire-server/main.go

# Production Stage
FROM alpine:3.19
WORKDIR /app

# Copy the compiled binary
COPY --from=builder /app/retire-server .

# Copy rbac-manifest.json required on startup
COPY --from=builder /app/rbac-manifest.json .

# Expose the service port
EXPOSE 8082

# Environment defaults
ENV PORT=8082

# Run the server
CMD ["./retire-server"]
