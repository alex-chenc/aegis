FROM alpine:latest

WORKDIR /root/

RUN apk add --no-cache ca-certificates

COPY bin/server .
COPY config/server.yaml config/

EXPOSE 8083
EXPOSE 19090

CMD ["./server"]