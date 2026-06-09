FROM golang:alpine AS builder

WORKDIR /app

# Install git for downloading dependencies
RUN apk add --no-cache git

# Download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY . .

# Build the application (modernc.org/sqlite is pure Go, so CGO is not strictly needed, but let's disable it just in case)
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o disbox .

FROM alpine:latest

# Install CA certificates and tzdata for SSL and timezones
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/disbox /app/disbox

# Expose the default port
EXPOSE 8080

# Run the binary
CMD ["/app/disbox"]
