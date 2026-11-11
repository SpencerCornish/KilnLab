# Stage 1: build the React SPA into internal/webembed/dist
FROM node:24-alpine AS frontend
RUN npm install -g pnpm@11.1.2
WORKDIR /app/web
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm run build

# Stage 2: build the Go binary with the SPA embedded
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/internal/webembed/dist ./internal/webembed/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o kilnlab ./cmd/server

# Stage 3: minimal runtime
FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && adduser -D -H kilnlab
WORKDIR /app
COPY --from=builder /app/kilnlab .
USER kilnlab
EXPOSE 8080
ENTRYPOINT ["./kilnlab"]
