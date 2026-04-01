FROM alpine:latest

WORKDIR /root/

RUN apk add --no-cache ca-certificates

COPY bin/dc .
COPY config/dc.yaml config/

EXPOSE 19092

CMD ["./dc"]