FROM alpine:latest

WORKDIR /root/

RUN apk add --no-cache ca-certificates

COPY bin/api-server .
COPY config/api-server.yaml config/

EXPOSE 8082
EXPOSE 19093

CMD ["./api-server"]