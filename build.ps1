Param(
    [string]$Version = "1.0.0",
    [ValidateSet("windows", "linux", "all")]
    [string]$Target = "windows",
    [string]$Arch = "amd64"
)

$ErrorActionPreference = "Stop"
$ldflags = "-s -w -X main.version=$Version"
$outDir = "bin"

if (!(Test-Path $outDir)) {
    New-Item -ItemType Directory -Path $outDir | Out-Null
}

$origGOOS = $env:GOOS
$origGOARCH = $env:GOARCH
$origCGO = $env:CGO_ENABLED

try {
    $env:CGO_ENABLED = "0"

    if ($Target -eq "windows" -or $Target -eq "all") {
        $env:GOOS = "windows"
        $env:GOARCH = $Arch
        Write-Host "==> Building Windows binaries (version $Version, $Arch)..." -ForegroundColor Cyan
        go build -ldflags $ldflags -trimpath -o "$outDir/server.exe" ./cmd/server
        go build -ldflags $ldflags -trimpath -o "$outDir/client.exe" ./cmd/client
        Write-Host "    [OK] $outDir/server.exe, $outDir/client.exe" -ForegroundColor Green
    }

    if ($Target -eq "linux" -or $Target -eq "all") {
        $env:GOOS = "linux"
        $env:GOARCH = $Arch
        Write-Host "==> Building Linux binaries (version $Version, $Arch)..." -ForegroundColor Cyan
        go build -ldflags $ldflags -trimpath -o "$outDir/server-linux" ./cmd/server
        go build -ldflags $ldflags -trimpath -o "$outDir/client-linux" ./cmd/client
        Write-Host "    [OK] $outDir/server-linux, $outDir/client-linux" -ForegroundColor Green
    }
}
finally {
    $env:GOOS = $origGOOS
    $env:GOARCH = $origGOARCH
    $env:CGO_ENABLED = $origCGO
}

Write-Host "==> Build complete!" -ForegroundColor Green
