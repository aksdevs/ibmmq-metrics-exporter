#!/bin/bash
# Docker build script with configurable IBM MQ paths
# This script demonstrates how to build the Docker image with different MQ configurations

set -e

# Default values
USE_LOCAL_MQ=${USE_LOCAL_MQ:-false}
MQ_INCLUDE_PATH=${MQ_INCLUDE_PATH:-/opt/mqm/inc}
MQ_LIB_PATH=${MQ_LIB_PATH:-/opt/mqm/lib64}
IMAGE_TAG=${IMAGE_TAG:-ibmmq-collector:latest}
TARGET_ARCH=${TARGET_ARCH:-amd64}
FORCE_64BIT=${FORCE_64BIT:-true}
CUSTOM_CC=${CUSTOM_CC:-}
CUSTOM_CXX=${CUSTOM_CXX:-}
EXTRA_CFLAGS=${EXTRA_CFLAGS:-}
EXTRA_LDFLAGS=${EXTRA_LDFLAGS:-}

# Platform-specific defaults
if [[ "$OSTYPE" == "msys" || "$OSTYPE" == "cygwin" ]]; then
    # Windows defaults
    DEFAULT_MQ_INCLUDE="C:/Program Files/IBM/MQ/tools/c/include"
    DEFAULT_MQ_LIB="C:/Program Files/IBM/MQ/bin64"
elif [[ "$OSTYPE" == "darwin"* ]]; then
    # macOS defaults
    DEFAULT_MQ_INCLUDE="/opt/mqm/inc"
    DEFAULT_MQ_LIB="/opt/mqm/lib64"
else
    # Linux defaults
    DEFAULT_MQ_INCLUDE="/opt/mqm/inc"
    DEFAULT_MQ_LIB="/opt/mqm/lib64"
fi

usage() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Options:"
    echo "  --local-mq              Use local IBM MQ installation"
    echo "  --include-path PATH     IBM MQ include path (default: platform-specific)"
    echo "  --lib-path PATH         IBM MQ library path (default: platform-specific)"
    echo "  --tag TAG               Docker image tag (default: ibmmq-collector:latest)"
    echo "  --help                  Show this help"
    echo ""
    echo "Platform defaults:"
    echo "  Windows: Include: 'C:/Program Files/IBM/MQ/tools/c/include'"
    echo "           Library: 'C:/Program Files/IBM/MQ/bin64'"
    echo "  Linux:   Include: '/opt/mqm/inc'"
    echo "           Library: '/opt/mqm/lib64'"
    echo ""
    echo "Examples:"
    echo "  # Build with downloaded MQ client (default)"
    echo "  $0"
    echo ""
    echo "  # Build with local Windows MQ installation"
    echo "  $0 --local-mq --include-path 'C:/Program Files/IBM/MQ/tools/c/include' --lib-path 'C:/Program Files/IBM/MQ/bin64'"
    echo ""
    echo "  # Build with local Linux MQ installation"
    echo "  $0 --local-mq --include-path '/opt/mqm/inc' --lib-path '/opt/mqm/lib64'"
    echo ""
    echo "  # Build with custom paths"
    echo "  $0 --local-mq --include-path '/usr/local/include/mq' --lib-path '/usr/local/lib/mq'"
    exit 0
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --local-mq)
            USE_LOCAL_MQ=true
            shift
            ;;
        --include-path)
            MQ_INCLUDE_PATH="$2"
            shift 2
            ;;
        --lib-path)
            MQ_LIB_PATH="$2"
            shift 2
            ;;
        --tag)
            IMAGE_TAG="$2"
            shift 2
            ;;
        --help)
            usage
            ;;
        *)
            echo "Unknown option: $1"
            usage
            ;;
    esac
done

# Use platform defaults if local MQ is requested but paths not specified
if [[ "$USE_LOCAL_MQ" == "true" ]]; then
    if [[ "$MQ_INCLUDE_PATH" == "/opt/mqm/inc" ]]; then
        MQ_INCLUDE_PATH="$DEFAULT_MQ_INCLUDE"
    fi
    if [[ "$MQ_LIB_PATH" == "/opt/mqm/lib64" ]]; then
        MQ_LIB_PATH="$DEFAULT_MQ_LIB"
    fi
fi

echo "Building Docker image with configuration:"
echo "  Image tag: $IMAGE_TAG"
echo "  Use local MQ: $USE_LOCAL_MQ"
echo "  Include path: $MQ_INCLUDE_PATH"
echo "  Library path: $MQ_LIB_PATH"
echo ""

# Enable BuildKit
export DOCKER_BUILDKIT=1

# Build Docker image with build arguments
docker build \
    --build-arg USE_LOCAL_MQ="$USE_LOCAL_MQ" \
    --build-arg MQ_INCLUDE_PATH="$MQ_INCLUDE_PATH" \
    --build-arg MQ_LIB_PATH="$MQ_LIB_PATH" \
    --build-arg MQ_HOST_INCLUDE_PATH="$MQ_INCLUDE_PATH" \
    --build-arg MQ_HOST_LIB_PATH="$MQ_LIB_PATH" \
    --tag "$IMAGE_TAG" \
    --progress=plain \
    .

echo ""
echo "Docker image built successfully: $IMAGE_TAG"
echo ""
echo "To run the container:"
echo "docker run --rm -p 9090:9090 $IMAGE_TAG"