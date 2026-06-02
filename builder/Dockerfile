ARG EBPF_BASE_IMAGE=aegis-agent-builder-ubi8:5.8.0

FROM ${EBPF_BASE_IMAGE} AS app-builder

WORKDIR /src/builder
COPY builder/go.mod builder/go.sum ./
RUN go mod download
COPY builder/ ./
RUN GOMAXPROCS=1 CGO_ENABLED=0 GOOS=linux go build -p=1 -o /out/builder ./cmd/...

FROM ${EBPF_BASE_IMAGE}

WORKDIR /opt/aegis/builder
ARG BUILDER_IMAGE_DIGEST=""
ENV BUILDER_IMAGE_DIGEST=${BUILDER_IMAGE_DIGEST} \
    AEGIS_EBPF_INCLUDE=/opt/aegis/ebpf/include

COPY --from=app-builder /out/builder ./builder

EXPOSE 19096
CMD ["./builder"]
