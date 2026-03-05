# --- Build Stage ---
FROM golang:1.25-alpine AS builder
WORKDIR /src
ENV GOPROXY=https://goproxy.cn,direct
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /app/server ./cmd/main.go

# --- Final Stage ---
FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/server .
# HTTP port
EXPOSE 8080
# gRPC port
EXPOSE 9090
CMD ["./server"]