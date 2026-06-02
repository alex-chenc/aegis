FROM registry.access.redhat.com/ubi8/ubi:8.10

ARG GO_VERSION=1.25.0
ARG TARGETARCH

ENV GOROOT=/usr/local/go \
    GOPROXY=https://goproxy.cn,direct \
    AEGIS_EBPF_INCLUDE=/opt/aegis/ebpf/include \
    PATH=/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

RUN set -eux; \
    dnf -y --setopt=install_weak_deps=False install \
        ca-certificates \
        clang \
        curl \
        file \
        findutils \
        gzip \
        kernel-headers \
        llvm \
        make \
        tar \
        which; \
    dnf -y --setopt=install_weak_deps=False install bpftool || true; \
    dnf clean all; \
    rm -rf /var/cache/dnf

RUN set -eux; \
    case "${TARGETARCH:-amd64}" in \
        amd64) go_arch=amd64 ;; \
        arm64) go_arch=arm64 ;; \
        *) echo "unsupported TARGETARCH=${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${go_arch}.tar.gz" -o /tmp/go.tgz; \
    rm -rf /usr/local/go; \
    tar -C /usr/local -xzf /tmp/go.tgz; \
    rm -f /tmp/go.tgz; \
    go version

RUN mkdir -p "${AEGIS_EBPF_INCLUDE}" /usr/include/aegis-ebpf

COPY agent/internal/ebpf/bpf/common.h "${AEGIS_EBPF_INCLUDE}/common.h"
COPY agent/internal/ebpf/bpf/event_output.h "${AEGIS_EBPF_INCLUDE}/event_output.h"
COPY agent/internal/ebpf/bpf/vmlinux.h "${AEGIS_EBPF_INCLUDE}/vmlinux.h"
COPY docker/ebpf-builder-base/include/bpf/ /usr/include/bpf/

RUN set -eux; \
    ln -sf "${AEGIS_EBPF_INCLUDE}/common.h" /usr/include/aegis-ebpf/common.h; \
    ln -sf "${AEGIS_EBPF_INCLUDE}/event_output.h" /usr/include/aegis-ebpf/event_output.h; \
    ln -sf "${AEGIS_EBPF_INCLUDE}/vmlinux.h" /usr/include/aegis-ebpf/vmlinux.h; \
    clang --version; \
    bpftool version || true; \
    test -f "${AEGIS_EBPF_INCLUDE}/common.h"; \
    test -f "${AEGIS_EBPF_INCLUDE}/event_output.h"; \
    test -f "${AEGIS_EBPF_INCLUDE}/vmlinux.h"; \
    test -f /usr/include/bpf/bpf_helpers.h; \
    test -f /usr/include/bpf/bpf_tracing.h; \
    test -f /usr/include/bpf/bpf_endian.h
