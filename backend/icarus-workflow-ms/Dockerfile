FROM golang:1.26.3-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o workflow-server cmd/workflow-server/main.go

FROM alpine:latest
WORKDIR /app
RUN mkdir -p /app/data
COPY --from=builder /app/workflow-server .
COPY --from=builder /app/env/.env.example ./env/.env
EXPOSE 8082
CMD ["./workflow-server"]
