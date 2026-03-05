FROM golang:1.25-alpine AS builder

WORKDIR /src
ENV GOPROXY=https://goproxy.cn,direct
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /app/agent .

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/agent .
RUN apk add --no-cache bash curl
CMD ["./agent", "-server_addr", "backend:9090", "-token", "a_shared_secret_token_for_agents"]