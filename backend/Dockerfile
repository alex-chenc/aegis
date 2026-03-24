FROM alpine:latest

WORKDIR /root/

RUN apk add --no-cache ca-certificates

COPY backend/backend .

RUN mkdir -p config
COPY backend/config/config.yaml config/
COPY backend/config/rules/ config/rules/

EXPOSE 8080
EXPOSE 9090

CMD ["./backend"]
