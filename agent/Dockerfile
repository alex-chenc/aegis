ARG EBPF_BASE_IMAGE=aegis-agent-builder-ubi8:5.8.0

FROM ${EBPF_BASE_IMAGE} AS build

WORKDIR /src/agent
COPY go.mod go.sum ./
RUN go mod download
COPY . .

ARG BPF_TARGET_ARCH=x86
RUN make clean && make BPF_TARGET_ARCH="${BPF_TARGET_ARCH}" BPF_TRANSPORT=all all

FROM registry.access.redhat.com/ubi8/ubi-minimal:8.10

WORKDIR /out
COPY --from=build /src/agent/dist/ /out/

CMD ["ls", "-l", "/out"]
