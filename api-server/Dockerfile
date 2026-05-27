FROM golang:1.25-alpine AS builder

WORKDIR /src

RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories && apk add --no-cache build-base

ENV GOPROXY=https://goproxy.cn,direct
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN GOMAXPROCS=1 CGO_ENABLED=0 GOOS=linux go build -p=1 -o /out/api-server ./cmd/...

FROM alpine:latest

WORKDIR /root/

RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories && apk add --no-cache ca-certificates

COPY --from=builder /out/api-server ./api-server
COPY config/api-server.yaml config/

EXPOSE 8082
EXPOSE 19093

CMD ["./api-server"]
