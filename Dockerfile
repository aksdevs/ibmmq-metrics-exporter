# syntax=docker/dockerfile:1.4
# Enable BuildKit for parallel builds and advanced features

# Build arguments for configurable IBM MQ paths and compiler settings
ARG MQ_INCLUDE_PATH="/opt/mqm/inc"
ARG MQ_LIB_PATH="/opt/mqm/lib64"
ARG MQ_HOST_INCLUDE_PATH=""
ARG MQ_HOST_LIB_PATH=""
ARG USE_LOCAL_MQ="false"

# Compiler configuration arguments - auto-detect 64-bit GCC or use custom
ARG TARGET_ARCH="amd64"
ARG FORCE_64BIT="true"
ARG CUSTOM_CC=""
ARG CUSTOM_CXX=""
ARG CGO_CFLAGS_EXTRA=""
ARG CGO_LDFLAGS_EXTRA=""

# Base image with Go and build tools
FROM golang:1.24-bullseye AS build-base

# Install system dependencies for IBM MQ client with 64-bit GCC
RUN apt-get update && apt-get install -y \
    build-essential \
    gcc-multilib \
    g++-multilib \
    libc6-dev \
    make \
    wget \
    curl \
    unzip \
    ca-certificates \
    file \
    && rm -rf /var/lib/apt/lists/*

# Verify 64-bit GCC is available
RUN gcc --version && gcc -dumpmachine && file /usr/bin/gcc

# Set up build environment for 64-bit
ENV CGO_ENABLED=1
ENV GOOS=linux
ENV GOARCH=amd64
ENV CC=gcc
ENV CXX=g++

# Create app directory
WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./

# Download dependencies (this layer will be cached)
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# IBM MQ Client stage
FROM build-base as mq-client

# Pass build arguments to this stage
ARG MQ_INCLUDE_PATH
ARG MQ_LIB_PATH
ARG MQ_HOST_INCLUDE_PATH
ARG MQ_HOST_LIB_PATH
ARG USE_LOCAL_MQ
ARG MQ_VERSION=9.3.4.1
ARG MQ_PACKAGE="9.3.4.1-IBM-MQC-Redist-LinuxX64.tar.gz"

# Set up IBM MQ client libraries based on configuration
RUN --mount=type=cache,target=/tmp/mqclient \
    echo "IBM MQ Setup Configuration:" && \
    echo "  USE_LOCAL_MQ: ${USE_LOCAL_MQ}" && \
    echo "  MQ_INCLUDE_PATH: ${MQ_INCLUDE_PATH}" && \
    echo "  MQ_LIB_PATH: ${MQ_LIB_PATH}" && \
    echo "  MQ_HOST_INCLUDE_PATH: ${MQ_HOST_INCLUDE_PATH}" && \
    echo "  MQ_HOST_LIB_PATH: ${MQ_HOST_LIB_PATH}" && \
    mkdir -p "${MQ_INCLUDE_PATH}" "${MQ_LIB_PATH}" && \
    if [ "${USE_LOCAL_MQ}" = "true" ] && [ -n "${MQ_HOST_INCLUDE_PATH}" ] && [ -n "${MQ_HOST_LIB_PATH}" ]; then \
    echo "Using local IBM MQ installation..." && \
    echo "Note: Copy MQ files using bind mounts or COPY --from during build" && \
    echo "Example: docker build --build-arg USE_LOCAL_MQ=true --build-arg MQ_HOST_INCLUDE_PATH=/host/mq/inc ..." && \
    mkdir -p "${MQ_INCLUDE_PATH}" "${MQ_LIB_PATH}"; \
    else \
    echo "Attempting to download IBM MQ Client ${MQ_VERSION}..." && \
    cd /tmp/mqclient && \
    (wget -v --timeout=30 --tries=3 "https://public.dhe.ibm.com/ibmdl/export/pub/software/websphere/messaging/mqdev/redist/${MQ_PACKAGE}" || \
    wget -v --timeout=30 --tries=3 "https://public.dhe.ibm.com/ibmdl/export/pub/software/websphere/messaging/mqdev/redist/9.3.4.1-IBM-MQC-Redist-LinuxX64.tar.gz" || \
    echo "Download failed, will create stub libraries...") && \
    if ls *.tar.gz 1> /dev/null 2>&1; then \
    echo "Extracting IBM MQ Client..." && \
    tar -xzf *.tar.gz && \
    if [ -d mqm ]; then \
    cp -r mqm/inc/* "${MQ_INCLUDE_PATH}/" 2>/dev/null || echo "No include files found"; \
    cp -r mqm/lib64/* "${MQ_LIB_PATH}/" 2>/dev/null || echo "No lib files found"; \
    fi; \
    else \
    echo "Creating stub MQ libraries for build compatibility..." && \
    echo '#ifndef CMQC_H' > "${MQ_INCLUDE_PATH}/cmqc.h" && \
    echo '#define CMQC_H' >> "${MQ_INCLUDE_PATH}/cmqc.h" && \
    echo '#define MQCC_OK 0' >> "${MQ_INCLUDE_PATH}/cmqc.h" && \
    echo '#define MQRC_NONE 0' >> "${MQ_INCLUDE_PATH}/cmqc.h" && \
    echo 'typedef long MQLONG;' >> "${MQ_INCLUDE_PATH}/cmqc.h" && \
    echo 'typedef char MQCHAR;' >> "${MQ_INCLUDE_PATH}/cmqc.h" && \
    echo '#endif' >> "${MQ_INCLUDE_PATH}/cmqc.h" && \
    echo '#ifndef CMQCFC_H' > "${MQ_INCLUDE_PATH}/cmqcfc.h" && \
    echo '#define CMQCFC_H' >> "${MQ_INCLUDE_PATH}/cmqcfc.h" && \
    echo '#include "cmqc.h"' >> "${MQ_INCLUDE_PATH}/cmqcfc.h" && \
    echo '#endif' >> "${MQ_INCLUDE_PATH}/cmqcfc.h" && \
    echo 'void dummy() {}' > /tmp/dummy.c && \
    gcc -shared -o "${MQ_LIB_PATH}/libmqm.so" /tmp/dummy.c; \
    fi; \
    fi && \
    # Set up library path
    echo "${MQ_LIB_PATH}" > /etc/ld.so.conf.d/mqm.conf && \
    ldconfig && \
    # Verify installation
    echo "IBM MQ files installed:" && \
    ls -la "${MQ_INCLUDE_PATH}/" && \
    ls -la "${MQ_LIB_PATH}/"

# Build stage
FROM mq-client as builder

# Pass build arguments to this stage
ARG MQ_INCLUDE_PATH
ARG MQ_LIB_PATH
ARG TARGET_ARCH
ARG FORCE_64BIT
ARG CUSTOM_CC
ARG CUSTOM_CXX
ARG CGO_CFLAGS_EXTRA
ARG CGO_LDFLAGS_EXTRA

# Auto-detect and configure 64-bit GCC compiler
RUN echo "=== Compiler Configuration ===" && \
    echo "Target Architecture: ${TARGET_ARCH}" && \
    echo "Force 64-bit: ${FORCE_64BIT}" && \
    echo "Custom CC: ${CUSTOM_CC}" && \
    echo "Custom CXX: ${CUSTOM_CXX}" && \
    echo "" && \
    # Check available compilers and their capabilities
    echo "Available compilers:" && \
    which gcc && gcc --version && gcc -dumpmachine && \
    (which x86_64-linux-gnu-gcc && x86_64-linux-gnu-gcc --version && x86_64-linux-gnu-gcc -dumpmachine) || echo "x86_64-linux-gnu-gcc not found" && \
    echo "" && \
    # Test 64-bit compilation capability
    echo "Testing 64-bit compilation support..." && \
    echo 'int main() { return sizeof(void*) == 8 ? 0 : 1; }' > /tmp/test64.c && \
    if gcc -m64 -o /tmp/test64 /tmp/test64.c 2>/dev/null && /tmp/test64; then \
    echo "✓ Default gcc supports 64-bit compilation with -m64" && \
    echo "COMPILER_64BIT_SUPPORT=gcc-m64" > /tmp/compiler_config; \
    elif x86_64-linux-gnu-gcc -o /tmp/test64 /tmp/test64.c 2>/dev/null && /tmp/test64; then \
    echo "✓ x86_64-linux-gnu-gcc available for 64-bit compilation" && \
    echo "COMPILER_64BIT_SUPPORT=x86_64-linux-gnu-gcc" > /tmp/compiler_config; \
    elif gcc -o /tmp/test64 /tmp/test64.c 2>/dev/null && /tmp/test64; then \
    echo "✓ Default gcc produces 64-bit binaries by default" && \
    echo "COMPILER_64BIT_SUPPORT=gcc-default" > /tmp/compiler_config; \
    else \
    echo "⚠ No 64-bit compilation support detected" && \
    echo "COMPILER_64BIT_SUPPORT=none" > /tmp/compiler_config; \
    fi && \
    # Set up compiler based on detection results and configuration
    source /tmp/compiler_config && \
    if [ -n "${CUSTOM_CC}" ] && [ -n "${CUSTOM_CXX}" ]; then \
    echo "Using custom compilers: CC=${CUSTOM_CC}, CXX=${CUSTOM_CXX}" && \
    echo "CC=${CUSTOM_CC}" > /tmp/final_compiler_config && \
    echo "CXX=${CUSTOM_CXX}" >> /tmp/final_compiler_config; \
    elif [ "${COMPILER_64BIT_SUPPORT}" = "gcc-m64" ]; then \
    echo "Using default gcc with -m64 flag" && \
    echo "CC=gcc" > /tmp/final_compiler_config && \
    echo "CXX=g++" >> /tmp/final_compiler_config && \
    echo "ARCH_FLAGS=-m64" >> /tmp/final_compiler_config; \
    elif [ "${COMPILER_64BIT_SUPPORT}" = "x86_64-linux-gnu-gcc" ]; then \
    echo "Using x86_64-linux-gnu-gcc" && \
    echo "CC=x86_64-linux-gnu-gcc" > /tmp/final_compiler_config && \
    echo "CXX=x86_64-linux-gnu-g++" >> /tmp/final_compiler_config && \
    echo "ARCH_FLAGS=" >> /tmp/final_compiler_config; \
    elif [ "${COMPILER_64BIT_SUPPORT}" = "gcc-default" ]; then \
    echo "Using default gcc (already 64-bit)" && \
    echo "CC=gcc" > /tmp/final_compiler_config && \
    echo "CXX=g++" >> /tmp/final_compiler_config && \
    echo "ARCH_FLAGS=" >> /tmp/final_compiler_config; \
    else \
    echo "Falling back to default gcc" && \
    echo "CC=gcc" > /tmp/final_compiler_config && \
    echo "CXX=g++" >> /tmp/final_compiler_config && \
    echo "ARCH_FLAGS=" >> /tmp/final_compiler_config; \
    fi && \
    cat /tmp/final_compiler_config

# Set environment variables based on detected compiler configuration
RUN source /tmp/final_compiler_config && \
    echo "Final compiler configuration:" && \
    echo "  CC: $CC" && \
    echo "  CXX: $CXX" && \
    echo "  ARCH_FLAGS: $ARCH_FLAGS" && \
    echo "  MQ_INCLUDE_PATH: ${MQ_INCLUDE_PATH}" && \
    echo "  MQ_LIB_PATH: ${MQ_LIB_PATH}" && \
    # Create CGO environment script to handle paths with spaces properly
    echo "#!/bin/bash" > /tmp/setup_cgo.sh && \
    echo "export CC=\"$CC\"" >> /tmp/setup_cgo.sh && \
    echo "export CXX=\"$CXX\"" >> /tmp/setup_cgo.sh && \
    echo "export CGO_ENABLED=1" >> /tmp/setup_cgo.sh && \
    echo "export GOARCH=${TARGET_ARCH}" >> /tmp/setup_cgo.sh && \
    if [ -n "$ARCH_FLAGS" ]; then \
    echo "export CGO_CFLAGS=\"-I${MQ_INCLUDE_PATH} $ARCH_FLAGS ${CGO_CFLAGS_EXTRA}\"" >> /tmp/setup_cgo.sh && \
    echo "export CGO_LDFLAGS=\"-L${MQ_LIB_PATH} -lmqm $ARCH_FLAGS ${CGO_LDFLAGS_EXTRA}\"" >> /tmp/setup_cgo.sh; \
    else \
    echo "export CGO_CFLAGS=\"-I${MQ_INCLUDE_PATH} ${CGO_CFLAGS_EXTRA}\"" >> /tmp/setup_cgo.sh && \
    echo "export CGO_LDFLAGS=\"-L${MQ_LIB_PATH} -lmqm ${CGO_LDFLAGS_EXTRA}\"" >> /tmp/setup_cgo.sh; \
    fi && \
    chmod +x /tmp/setup_cgo.sh && \
    cat /tmp/setup_cgo.sh

# Verify final compiler setup
RUN source /tmp/setup_cgo.sh && \
    echo "=== Final Compiler Verification ===" && \
    echo "CC: $CC" && \
    echo "CXX: $CXX" && \
    echo "CGO_CFLAGS: $CGO_CFLAGS" && \
    echo "CGO_LDFLAGS: $CGO_LDFLAGS" && \
    $CC --version && \
    $CC -dumpmachine && \
    # Test final compilation
    echo 'int main() { return sizeof(void*) == 8 ? 0 : 1; }' > /tmp/final_test.c && \
    if echo "$CGO_CFLAGS" | grep -q "\-m64"; then \
    $CC $($echo "$CGO_CFLAGS" | grep -o "\-m64") -o /tmp/final_test /tmp/final_test.c && /tmp/final_test && echo "✓ 64-bit compilation verified"; \
    else \
    $CC -o /tmp/final_test /tmp/final_test.c && /tmp/final_test && echo "✓ Compilation verified"; \
    fi

# Copy source code
COPY . .

# Build both collector and pcf-dumper using configured compiler environment
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    echo "=== Building Applications ===" && \
    source /tmp/setup_cgo.sh && \
    echo "Build environment:" && \
    echo "  CC: $CC" && \
    echo "  CXX: $CXX" && \
    echo "  CGO_ENABLED: $CGO_ENABLED" && \
    echo "  GOARCH: $GOARCH" && \
    echo "  CGO_CFLAGS: $CGO_CFLAGS" && \
    echo "  CGO_LDFLAGS: $CGO_LDFLAGS" && \
    echo "" && \
    echo "Building collector..." && \
    go build -ldflags="-w -s" -o /app/bin/collector ./cmd/collector && \
    echo "✓ Collector built successfully" && \
    echo "" && \
    echo "Building pcf-dumper..." && \
    go build -ldflags="-w -s" -o /app/bin/pcf-dumper ./cmd/pcf-dumper && \
    echo "✓ PCF dumper built successfully" && \
    echo "" && \
    echo "=== Binary Verification ===" && \
    file /app/bin/collector /app/bin/pcf-dumper && \
    ls -la /app/bin/ && \
    echo "✓ Build completed successfully"

# Test stage (runs in parallel with build)
FROM builder as test
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go test -v ./pkg/config ./pkg/pcf

# Runtime base
FROM debian:bullseye-slim as runtime-base

# Install minimal runtime dependencies
RUN apt-get update && apt-get install -y \
    ca-certificates \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Pass build arguments to runtime stage
ARG MQ_LIB_PATH

# Copy MQ client libraries from build stage using configurable paths
COPY --from=mq-client ${MQ_LIB_PATH} ${MQ_LIB_PATH}
COPY --from=mq-client /etc/ld.so.conf.d/mqm.conf /etc/ld.so.conf.d/mqm.conf

# Update library cache
RUN ldconfig

# Create non-root user for security
RUN groupadd -r ibmmq && useradd -r -g ibmmq -s /bin/false ibmmq

# Final stage
FROM runtime-base as final

# Copy binaries from builder
COPY --from=builder /app/bin/collector /usr/local/bin/collector
COPY --from=builder /app/bin/pcf-dumper /usr/local/bin/pcf-dumper

# Copy default configuration
COPY configs/default.yaml /etc/ibmmq-collector/config.yaml

# Set up directories with proper permissions
RUN mkdir -p /var/log/ibmmq-collector /var/lib/ibmmq-collector && \
    chown -R ibmmq:ibmmq /var/log/ibmmq-collector /var/lib/ibmmq-collector

# Set environment variables
ENV PATH="/usr/local/bin:$PATH"
ENV CONFIG_PATH="/etc/ibmmq-collector/config.yaml"

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:9090/metrics || exit 1

# Expose Prometheus metrics port
EXPOSE 9090

# Switch to non-root user
USER ibmmq

# Default command
ENTRYPOINT ["collector"]
CMD ["--config", "/etc/ibmmq-collector/config.yaml", "--continuous"]

# Metadata
LABEL org.opencontainers.image.title="IBM MQ Statistics Collector"
LABEL org.opencontainers.image.description="Prometheus collector for IBM MQ statistics and accounting data"
LABEL org.opencontainers.image.vendor="IBM MQ Statistics Collector"
LABEL org.opencontainers.image.licenses="Apache-2.0"
LABEL org.opencontainers.image.source="https://github.com/atulksin/ibmmq-go-stat-otel"