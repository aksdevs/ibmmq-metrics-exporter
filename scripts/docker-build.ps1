# Docker build script with configurable IBM MQ paths
# PowerShell version for Windows

param(
    [switch]$LocalMQ = $false,
    [string]$IncludePath = "",
    [string]$LibPath = "",
    [string]$Tag = "ibmmq-collector:latest",
    [string]$TargetArch = "amd64",
    [switch]$Force64Bit = $true,
    [string]$CustomCC = "",
    [string]$CustomCXX = "",
    [string]$ExtraCFlags = "",
    [string]$ExtraLDFlags = "",
    [switch]$Help = $false
)

function Show-Usage {
    Write-Host "Usage: .\docker-build.ps1 [OPTIONS]"
    Write-Host ""
    Write-Host "IBM MQ Options:"
    Write-Host "  -LocalMQ              Use local IBM MQ installation"
    Write-Host "  -IncludePath PATH     IBM MQ include path"
    Write-Host "  -LibPath PATH         IBM MQ library path"
    Write-Host ""
    Write-Host "Compiler Options:"
    Write-Host "  -TargetArch ARCH      Target architecture (default: amd64)"
    Write-Host "  -Force64Bit           Force 64-bit compilation (default: true)"
    Write-Host "  -CustomCC PATH        Custom C compiler path"
    Write-Host "  -CustomCXX PATH       Custom C++ compiler path"
    Write-Host "  -ExtraCFlags FLAGS    Additional CGO_CFLAGS"
    Write-Host "  -ExtraLDFlags FLAGS   Additional CGO_LDFLAGS"
    Write-Host ""
    Write-Host "General Options:"
    Write-Host "  -Tag TAG              Docker image tag (default: ibmmq-collector:latest)"
    Write-Host "  -Help                 Show this help"
    Write-Host ""
    Write-Host "Platform defaults:"
    Write-Host "  Windows: Include: 'C:\Program Files\IBM\MQ\tools\c\include'"
    Write-Host "           Library: 'C:\Program Files\IBM\MQ\bin64'"
    Write-Host ""
    Write-Host "Examples:"
    Write-Host "  # Build with auto-detected 64-bit GCC (default)"
    Write-Host "  .\docker-build.ps1"
    Write-Host ""
    Write-Host "  # Build with local Windows MQ installation"
    Write-Host "  .\docker-build.ps1 -LocalMQ"
    Write-Host ""
    Write-Host "  # Build with custom compiler"
    Write-Host "  .\docker-build.ps1 -CustomCC 'x86_64-linux-gnu-gcc' -CustomCXX 'x86_64-linux-gnu-g++'"
    Write-Host ""
    Write-Host "  # Build for different architecture"
    Write-Host "  .\docker-build.ps1 -TargetArch 'arm64' -Force64Bit:$false"
    exit 0
}

if ($Help) {
    Show-Usage
}

# Set platform-specific defaults
$DefaultIncludePath = "C:\Program Files\IBM\MQ\tools\c\include"
$DefaultLibPath = "C:\Program Files\IBM\MQ\bin64"

# Use defaults if local MQ requested but paths not specified
if ($LocalMQ) {
    if ([string]::IsNullOrEmpty($IncludePath)) {
        $IncludePath = $DefaultIncludePath
    }
    if ([string]::IsNullOrEmpty($LibPath)) {
        $LibPath = $DefaultLibPath
    }
} else {
    # Set container defaults for downloaded MQ client
    if ([string]::IsNullOrEmpty($IncludePath)) {
        $IncludePath = "/opt/mqm/inc"
    }
    if ([string]::IsNullOrEmpty($LibPath)) {
        $LibPath = "/opt/mqm/lib64"
    }
}

Write-Host "Building Docker image with configuration:"
Write-Host "IBM MQ Configuration:"
Write-Host "  Use local MQ: $LocalMQ"
Write-Host "  Include path: $IncludePath"
Write-Host "  Library path: $LibPath"
Write-Host "Compiler Configuration:"
Write-Host "  Target architecture: $TargetArch"
Write-Host "  Force 64-bit: $Force64Bit"
Write-Host "  Custom CC: $(if ($CustomCC) { $CustomCC } else { 'auto-detect' })"
Write-Host "  Custom CXX: $(if ($CustomCXX) { $CustomCXX } else { 'auto-detect' })"
Write-Host "  Extra CFLAGS: $ExtraCFlags"
Write-Host "  Extra LDFLAGS: $ExtraLDFlags"
Write-Host "General:"
Write-Host "  Image tag: $Tag"
Write-Host ""

# Enable BuildKit
$env:DOCKER_BUILDKIT = 1

# Convert Windows paths to Unix-style for Docker context
$UnixIncludePath = $IncludePath -replace '\\', '/' -replace 'C:', '/c'
$UnixLibPath = $LibPath -replace '\\', '/' -replace 'C:', '/c'

Write-Host "Converted paths for Docker context:"
Write-Host "  Unix include path: $UnixIncludePath"
Write-Host "  Unix library path: $UnixLibPath"
Write-Host ""

# Build Docker image with build arguments
$BuildArgs = @(
    "--build-arg", "USE_LOCAL_MQ=$($LocalMQ.ToString().ToLower())"
    "--build-arg", "MQ_INCLUDE_PATH=$UnixIncludePath"
    "--build-arg", "MQ_LIB_PATH=$UnixLibPath"
    "--build-arg", "MQ_HOST_INCLUDE_PATH=$UnixIncludePath"
    "--build-arg", "MQ_HOST_LIB_PATH=$UnixLibPath"
    "--build-arg", "TARGET_ARCH=$TargetArch"
    "--build-arg", "FORCE_64BIT=$($Force64Bit.ToString().ToLower())"
    "--build-arg", "CUSTOM_CC=$CustomCC"
    "--build-arg", "CUSTOM_CXX=$CustomCXX"
    "--build-arg", "CGO_CFLAGS_EXTRA=$ExtraCFlags"
    "--build-arg", "CGO_LDFLAGS_EXTRA=$ExtraLDFlags"
    "--tag", "$Tag"
    "--progress=plain"
    "."
)

Write-Host "Executing: docker build $($BuildArgs -join ' ')"
Write-Host ""

& docker build @BuildArgs

if ($LASTEXITCODE -eq 0) {
    Write-Host ""
    Write-Host "Docker image built successfully: $Tag" -ForegroundColor Green
    Write-Host ""
    Write-Host "To run the container:"
    Write-Host "docker run --rm -p 9090:9090 $Tag"
} else {
    Write-Host ""
    Write-Host "Docker build failed with exit code: $LASTEXITCODE" -ForegroundColor Red
    exit $LASTEXITCODE
}