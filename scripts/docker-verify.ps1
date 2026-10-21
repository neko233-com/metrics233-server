# Compatibility entry point. Both paths exercise the actual Dockerfile release
# runtime; no generated BusyBox Dockerfile and no global prune.
param(
  [ValidateRange(1, 10)][int]$Repeat = 1,
  [string]$Image = '',
  # Retained for existing command lines; tests publish ephemeral loopback ports.
  [int]$Port = 0
)
$ErrorActionPreference = 'Stop'
if ($Port -ne 0) { Write-Host 'Validation uses isolated ephemeral loopback ports to avoid port collisions.' }
& (Join-Path $PSScriptRoot 'dockerfile_test.ps1') -Repeat $Repeat -Image $Image -Browser
