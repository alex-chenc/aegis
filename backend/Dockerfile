FROM alpine:latest

WORKDIR /root/

RUN apk add --no-cache ca-certificates

COPY backend .

RUN mkdir -p config
COPY config/config.yaml config/

EXPOSE 8080
EXPOSE 9090

CMD ["./backend"]
