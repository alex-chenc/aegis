# Aegis eBPF Builder Base Image

This image is the shared eBPF build environment for the V5.8 agent release and
dynamic DetectionPackage builder service.

The base is pinned to UBI 8.10 rather than `latest` so the build environment
stays on the RHEL 8 compatibility line. For production release builds, prefer
pinning the `FROM` image by digest after validating the toolchain.

Build it from the repository root:

```bash
docker build -f docker/ebpf-builder-base/Dockerfile -t aegis-agent-builder-ubi8:5.8.0 .
```

Then build the dependent images:

```bash
docker build -f agent/Dockerfile --build-arg EBPF_BASE_IMAGE=aegis-agent-builder-ubi8:5.8.0 -t aegis-agent-artifacts:local agent
docker compose build builder
```

The image provides Go, clang, llvm, make, Linux UAPI headers, vendored minimal
libbpf compile headers, and the shared Aegis eBPF headers under
`/opt/aegis/ebpf/include`. `bpftool` is installed when it is available from the
configured UBI repositories; unregistered UBI8 repositories do not always expose
that package, and eBPF object compilation does not require it.
