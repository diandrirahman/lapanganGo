FROM golang:1.26-alpine AS builder

WORKDIR /app

# Copy root level go modules if they exist, but backend has its own in apps/api
COPY apps/api/go.mod apps/api/go.sum ./apps/api/

WORKDIR /app/apps/api
RUN go mod download

# Copy the source code
WORKDIR /app
COPY apps/api/ ./apps/api/
COPY db/migrations/ ./db/migrations/

# Build the application
WORKDIR /app/apps/api
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/api_binary ./cmd/api

# Create a lightweight production image
FROM alpine:3.20
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy the binary and migrations
COPY --from=builder /app/api_binary .
COPY --from=builder /app/db/migrations ./db/migrations

EXPOSE 8080

CMD ["./api_binary"]
