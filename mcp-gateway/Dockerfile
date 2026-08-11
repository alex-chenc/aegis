FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/mcp-gateway ./cmd/mcp-gateway

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/mcp-gateway /mcp-gateway
USER nonroot:nonroot
EXPOSE 8084
ENTRYPOINT ["/mcp-gateway"]
