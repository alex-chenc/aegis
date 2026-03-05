ARG BASE_IMAGE=golang:1.20-alpine
FROM ${BASE_IMAGE} AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN make build

FROM alpine:latest

WORKDIR /root/

COPY --from=builder /app/backend .
COPY --from=builder /app/config/config.yaml .

EXPOSE 8080
EXPOSE 9090

CMD ["./backend"]