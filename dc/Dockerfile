FROM golang:1.25-alpine AS builder

WORKDIR /src

RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories && apk add --no-cache build-base

ENV GOPROXY=https://goproxy.cn,direct
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/dc ./cmd/...

FROM alpine:latest

WORKDIR /root/

RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories && apk add --no-cache ca-certificates

COPY --from=builder /out/dc ./dc
COPY config/dc.yaml config/

EXPOSE 19092

CMD ["./dc"]
