param(
  [ValidateRange(1, 10)][int]$Repeat = 2,
  # Reuse an explicitly supplied production image; this script never removes it.
  [string]$Image = '',
  [switch]$IncludeDatabaseIntegration,
  [switch]$Browser
)
$ErrorActionPreference = 'Stop'
$rootPath = Split-Path -Parent $PSScriptRoot
$originalEnabled = [Environment]::GetEnvironmentVariable('METRICSPANEL_DOCKER_RUNTIME_TEST', 'Process')
$originalImage = [Environment]::GetEnvironmentVariable('METRICSPANEL_RUNTIME_TEST_IMAGE', 'Process')
$originalBrowser = [Environment]::GetEnvironmentVariable('METRICSPANEL_DOCKER_BROWSER_TEST', 'Process')
$originalSQLBrowser = [Environment]::GetEnvironmentVariable('METRICSPANEL_DOCKER_SQL_BROWSER_TEST', 'Process')

Push-Location $rootPath
try {
  $dockerOS = docker info --format '{{.OSType}}'
  if ($LASTEXITCODE -ne 0 -or $dockerOS -ne 'linux') {
    throw 'Deep Docker validation requires a running Linux Docker Engine.'
  }
  $env:METRICSPANEL_DOCKER_RUNTIME_TEST = '1'
  if ($Image) { $env:METRICSPANEL_RUNTIME_TEST_IMAGE = $Image }
  else { Remove-Item Env:METRICSPANEL_RUNTIME_TEST_IMAGE -ErrorAction SilentlyContinue }
  $env:METRICSPANEL_DOCKER_BROWSER_TEST = if ($Browser) { '1' } else { '0' }
  $env:METRICSPANEL_DOCKER_SQL_BROWSER_TEST = if ($Browser -and $IncludeDatabaseIntegration) { '1' } else { '0' }
  $testPattern = if ($IncludeDatabaseIntegration) { '^TestDocker(ProductionRuntime|EndToEnd)$' } else { '^TestDockerProductionRuntime$' }
  Write-Host "Verifying the production Dockerfile runtime, repeat=$Repeat; database suite=$IncludeDatabaseIntegration"
  go test -tags=integration ./tests/integration -run $testPattern "-count=$Repeat" -v -timeout=30m
  if ($LASTEXITCODE -ne 0) { throw 'Deep Docker validation failed; inspect the redacted test output above.' }
} finally {
  [Environment]::SetEnvironmentVariable('METRICSPANEL_DOCKER_RUNTIME_TEST', $originalEnabled, 'Process')
  [Environment]::SetEnvironmentVariable('METRICSPANEL_RUNTIME_TEST_IMAGE', $originalImage, 'Process')
  [Environment]::SetEnvironmentVariable('METRICSPANEL_DOCKER_BROWSER_TEST', $originalBrowser, 'Process')
  [Environment]::SetEnvironmentVariable('METRICSPANEL_DOCKER_SQL_BROWSER_TEST', $originalSQLBrowser, 'Process')
  Pop-Location
}
