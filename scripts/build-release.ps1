param(
  [string]$Version = "0.1.1",
  [string[]]$Targets = @("windows/amd64", "linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64"),
  [string]$OutputDirectory = ""
)
$ErrorActionPreference = "Stop"
$releaseArguments = @((Join-Path $PSScriptRoot 'build-release.mjs'), '--version', $Version, '--targets', ($Targets -join ','))
if ($OutputDirectory) { $releaseArguments += @('--output', $OutputDirectory) }
& node @releaseArguments
if ($LASTEXITCODE -ne 0) { throw 'Release build failed; existing artifacts were preserved.' }
