FROM golang:1.25-alpine AS builder

WORKDIR /src
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories && apk add --no-cache build-base clang llvm linux-headers
ENV GOPROXY=https://goproxy.cn,direct
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN GOMAXPROCS=1 CGO_ENABLED=0 GOOS=linux go build -p=1 -o /out/builder ./cmd/...

FROM alpine:latest
WORKDIR /root/
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories && apk add --no-cache ca-certificates clang llvm linux-headers
ARG BUILDER_IMAGE_DIGEST=""
ENV BUILDER_IMAGE_DIGEST=${BUILDER_IMAGE_DIGEST}
COPY --from=builder /out/builder ./builder
EXPOSE 19096
CMD ["./builder"]
