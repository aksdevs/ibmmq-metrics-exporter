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

# Base image with GCC compiler toolchain
FROM gcc:11-bullseye AS gcc-base

# Install Go 1.24 manually to have both GCC and Go in same image  
RUN wget -O go.tar.gz https://golang.org/dl/go1.24.0.linux-amd64.tar.gz && \
    tar -C /usr/local -xzf go.tar.gz && \
    rm go.tar.gz && \
    ln -sf /usr/local/go/bin/go /usr/bin/go && \
    ln -sf /usr/local/go/bin/gofmt /usr/bin/gofmt

# Set Go environment
ENV PATH="/usr/local/go/bin:${PATH}" \
    GOPATH="/go" \
    GOROOT="/usr/local/go"

# Create Go workspace
RUN mkdir -p /go/src /go/bin /go/pkg && chmod -R 777 /go

FROM gcc-base AS build-base

# Install additional build tools for 64-bit builds
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

# Verify GCC toolchain from gcc:11 image 
RUN echo "=== GCC Toolchain Verification ===" && \
    gcc --version && \
    gcc -dumpmachine && \
    file /usr/bin/gcc && \
    echo "Testing 64-bit compilation capability..." && \
    echo 'int main() { return sizeof(void*) == 8 ? 0 : 1; }' > /tmp/test64.c && \
    gcc -m64 -o /tmp/test64 /tmp/test64.c && /tmp/test64 && \
    echo "GCC 64-bit compilation verified"

# Set up build environment for static builds (no CGO)
ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

# Create app directory
WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./

# Download dependencies (this layer will be cached)
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# IBM MQ Developer Image stage - extract MQ client libraries and headers
FROM icr.io/ibm-messaging/mq:9.3.2.0-r2 AS mq-source

# IBM MQ Client preparation stage
FROM build-base AS mq-client

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
    { echo '#ifndef CMQC_H'; \
    echo '#define CMQC_H'; \
    echo 'typedef long MQLONG;'; \
    echo 'typedef unsigned long MQULONG;'; \
    echo 'typedef char MQCHAR;'; \
    echo 'typedef unsigned char MQBYTE;'; \
    echo 'typedef short MQINT16;'; \
    echo 'typedef long MQINT32;'; \
    echo 'typedef long long MQINT64;'; \
    echo 'typedef char MQINT8;'; \
    echo 'typedef float MQFLOAT32;'; \
    echo 'typedef double MQFLOAT64;'; \
    echo 'typedef void* MQPTR;'; \
    echo 'typedef char* PMQCHAR;'; \
    echo 'typedef long* PMQLONG;'; \
    echo 'typedef void* PMQVOID;'; \
    echo 'typedef void* PMQHMSG;'; \
    echo 'typedef MQLONG MQHCONN;'; \
    echo 'typedef MQLONG MQHOBJ;'; \
    echo 'typedef MQLONG MQHMSG;'; \
    echo '#define MQCC_OK 0'; \
    echo '#define MQCC_FAILED 2'; \
    echo '#define MQRC_NONE 0'; \
    echo '#define MQRC_HCONN_ERROR 2018'; \
    echo '#define MQRC_TRUNCATED_MSG_ACCEPTED 2079'; \
    echo '#define MQRC_PROPERTY_TYPE_ERROR 2411'; \
    echo '#define MQOO_FAIL_IF_QUIESCING 0x2000'; \
    echo '#define MQENC_NATIVE 0x00000222'; \
    echo '#define MQOT_TOPIC 8'; \
    echo '#define MQHM_NONE 0'; \
    echo '#define MQHM_UNUSABLE_HMSG -1'; \
    echo '#define MQHO_UNUSABLE_HOBJ -1'; \
    echo '#define MQCCSI_APPL -3'; \
    echo '#define MQ_OBJECT_NAME_LENGTH 48'; \
    echo '#define MQIA_FIRST 1001'; \
    echo '#define MQIA_LAST 2000'; \
    echo '#define MQIA_NAME_COUNT 1306'; \
    echo '#define MQCA_FIRST 2001'; \
    echo '#define MQCA_LAST 4000'; \
    echo '#define MQCA_NAMES 2021'; \
    echo '#define MQTYPE_BOOLEAN 1'; \
    echo '#define MQTYPE_INT8 2'; \
    echo '#define MQTYPE_INT16 3'; \
    echo '#define MQTYPE_INT32 4'; \
    echo '#define MQTYPE_INT64 5'; \
    echo '#define MQTYPE_FLOAT32 6'; \
    echo '#define MQTYPE_FLOAT64 7'; \
    echo '#define MQTYPE_STRING 8'; \
    echo '#define MQTYPE_BYTE_STRING 9'; \
    echo '#define MQTYPE_NULL 10'; \
    echo '#define sizeof_MQFLOAT32 4'; \
    echo '#define sizeof_MQFLOAT64 8'; \
    echo 'typedef struct { MQCHAR x[4]; } MQOD;'; \
    echo 'typedef struct { MQCHAR x[4]; } MQCNO;'; \
    echo 'typedef struct { MQCHAR x[4]; } MQMD;'; \
    echo 'typedef struct { MQCHAR x[4]; } MQGMO;'; \
    echo 'typedef struct { MQCHAR x[4]; } MQPMO;'; \
    echo 'typedef struct { MQCHAR x[4]; } MQSTS;'; \
    echo 'typedef struct { MQCHAR x[4]; } MQSD;'; \
    echo 'typedef struct { MQCHAR x[4]; } MQSRO;'; \
    echo '#define MQBO_NONE 0x00000000'; \
    echo 'typedef struct { MQCHAR x[4]; } MQBO;'; \
    echo 'typedef struct { MQCHAR x[4]; } MQCBC;'; \
    echo 'typedef struct { MQCHAR x[4]; } MQCMHO;'; \
    echo 'typedef struct { MQCHAR x[4]; } MQDMHO;'; \
    echo 'typedef struct { MQCHAR x[4]; } MQIMPO;'; \
    echo 'typedef struct { MQCHAR x[4]; } MQSMPO;'; \
    echo 'typedef struct { MQCHAR x[4]; } MQDMPO;'; \
    echo 'typedef struct { MQCHAR x[4]; } MQCHARV;'; \
    echo 'typedef struct { MQCHAR x[4]; } MQPD;'; \
    echo 'void MQBACK(); void MQBEGIN(); void MQCLOSE(); void MQCMIT(); void MQCONNX(); void MQCRTMH(); void MQDISC(); void MQDLTMH(); void MQDLTMP(); void MQGET(); void MQINQ(); void MQINQMP(); void MQOPEN(); void MQPUT(); void MQPUT1(); void MQSET(); void MQSETMP(); void MQSTAT(); void MQSUB(); void MQSUBRQ();'; \
    echo '#endif'; } > "${MQ_INCLUDE_PATH}/cmqc.h" && \
    echo '#ifndef CMQXC_H' > "${MQ_INCLUDE_PATH}/cmqxc.h" && \
    echo '#define CMQXC_H' >> "${MQ_INCLUDE_PATH}/cmqxc.h" && \
    echo '#include "cmqc.h"' >> "${MQ_INCLUDE_PATH}/cmqxc.h" && \
    echo '#define MQXCC_OK 0' >> "${MQ_INCLUDE_PATH}/cmqxc.h" && \
    echo '#endif' >> "${MQ_INCLUDE_PATH}/cmqxc.h" && \
    echo '#ifndef CMQCFC_H' > "${MQ_INCLUDE_PATH}/cmqcfc.h" && \
    echo '#define CMQCFC_H' >> "${MQ_INCLUDE_PATH}/cmqcfc.h" && \
    echo '#include "cmqc.h"' >> "${MQ_INCLUDE_PATH}/cmqcfc.h" && \
    echo '#include "cmqxc.h"' >> "${MQ_INCLUDE_PATH}/cmqcfc.h" && \
    echo '#endif' >> "${MQ_INCLUDE_PATH}/cmqcfc.h" && \
    echo '#ifndef CMQSTRC_H' > "${MQ_INCLUDE_PATH}/cmqstrc.h" && \
    echo '#define CMQSTRC_H' >> "${MQ_INCLUDE_PATH}/cmqstrc.h" && \
    echo '#include "cmqc.h"' >> "${MQ_INCLUDE_PATH}/cmqstrc.h" && \
    echo '#define MQSTRUC_ID_STRLEN 4' >> "${MQ_INCLUDE_PATH}/cmqstrc.h" && \
    echo '#endif' >> "${MQ_INCLUDE_PATH}/cmqstrc.h" && \
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
FROM mq-client AS builder

# Pass build arguments to this stage
ARG MQ_INCLUDE_PATH
ARG MQ_LIB_PATH
ARG TARGET_ARCH
ARG FORCE_64BIT
ARG CUSTOM_CC
ARG CUSTOM_CXX
ARG CGO_CFLAGS_EXTRA
ARG CGO_LDFLAGS_EXTRA

# Configure 64-bit GCC compiler (simplified since we use gcc:11 image)
RUN echo "=== GCC Configuration ===" && \
    echo "Using GCC from gcc:11 image with 64-bit compilation" && \
    gcc --version && \
    gcc -dumpmachine && \
    echo "Testing 64-bit compilation..." && \
    echo 'int main() { return sizeof(void*) == 8 ? 0 : 1; }' > /tmp/test64.c && \
    gcc -m64 -o /tmp/test64 /tmp/test64.c && /tmp/test64 && \
    echo "GCC 64-bit compilation confirmed" && \
    # Create compiler configuration
    echo "CC=gcc" > /tmp/final_compiler_config && \
    echo "CXX=g++" >> /tmp/final_compiler_config && \
    echo "ARCH_FLAGS=-m64" >> /tmp/final_compiler_config && \
    cat /tmp/final_compiler_config

# Create static build environment setup (no CGO)
RUN echo "=== Static Build Environment Setup ===" && \
    echo "Building static binaries without CGO for portability" && \
    echo "This avoids IBM MQ header dependencies at build time" && \
    echo "#!/bin/bash" > /tmp/setup_build.sh && \
    echo "export CGO_ENABLED=0" >> /tmp/setup_build.sh && \
    echo "export GOOS=linux" >> /tmp/setup_build.sh && \
    echo "export GOARCH=amd64" >> /tmp/setup_build.sh && \
    chmod +x /tmp/setup_build.sh && \
    cat /tmp/setup_build.sh

# Verify static build setup
RUN . /tmp/setup_build.sh && \
    echo "=== Static Build Verification ===" && \
    echo "CGO_ENABLED: $CGO_ENABLED" && \
    echo "GOOS: $GOOS" && \
    echo "GOARCH: $GOARCH" && \
    go version && \
    echo "Static build environment configured"

# Copy source code
COPY . .

# Build both collector and pcf-dumper using configured compiler environment
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    echo "=== Building Applications ===" && \
    . /tmp/setup_build.sh && \
    echo "Build environment:" && \
    echo "  CGO_ENABLED: $CGO_ENABLED" && \
    echo "  GOOS: $GOOS" && \
    echo "  GOARCH: $GOARCH" && \
    echo "" && \
    echo "Building collector..." && \
    go build -ldflags="-w -s" -o /app/bin/collector ./cmd/collector && \
    echo "Collector built successfully" && \
    echo "" && \
    echo "Building pcf-dumper..." && \
    go build -ldflags="-w -s" -o /app/bin/pcf-dumper ./cmd/pcf-dumper && \
    echo "PCF dumper built successfully" && \
    echo "" && \
    echo "=== Binary Verification ===" && \
    file /app/bin/collector /app/bin/pcf-dumper && \
    ls -la /app/bin/ && \
    echo "Build completed successfully"

# Test stage (runs in parallel with build)
FROM builder AS test
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go test -v ./pkg/config ./pkg/pcf

# Runtime base
FROM debian:bullseye-slim AS runtime-base

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
FROM runtime-base AS final

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
    CMD curl -f http://localhost:9091/metrics || exit 1

# Expose Prometheus metrics port
EXPOSE 9091

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
LABEL org.opencontainers.image.source="https://github.com/aksdevs/ibmmq-go-stat-otel"