# ─── Stage 1: Build Frontend (TypeScript + Vite) ───
FROM node:20-alpine AS frontend-builder

WORKDIR /app

COPY package.json package-lock.json ./
RUN npm ci

COPY tsconfig.json vite.config.ts ./
COPY public/ ./public/
COPY src/ ./src/

RUN npm run build

# ─── Stage 2: Build Go Backend ───
FROM golang:alpine AS go-builder

WORKDIR /app

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

# Copy backend source code
COPY . .

# Copy built frontend dist into proxy/dist
COPY --from=frontend-builder /app/proxy/dist ./proxy/dist

# Build the Go application
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o disbox .

# ─── Stage 3: Minimal Production Runtime ───
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=go-builder /app/disbox /app/disbox

EXPOSE 8080

CMD ["/app/disbox"]
