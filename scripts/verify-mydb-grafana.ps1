#Requires -Version 7.0
[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$MyDBMetricsURL,
    [Parameter(Mandatory)][string]$MyDBTokenFile,
    [Parameter(Mandatory)][string]$DashboardFile,
    [Parameter(Mandatory)][string]$MetricsPanelExe,
    [string]$OutputDirectory = (Join-Path $PSScriptRoot ('..\.cache\mydb-grafana-' + [guid]::NewGuid().ToString('N'))),
    [string]$GrafanaImage = 'grafana/grafana:13.2.3',
    [ValidateRange(0, 1200)][int]$BrowserHoldSeconds = 0
)
$ErrorActionPreference='Stop'
$root=[IO.Path]::GetFullPath($OutputDirectory)
if(Test-Path -LiteralPath $root){throw 'Use a new verification directory'}
New-Item -ItemType Directory -Path $root | Out-Null
$identity=[Security.Principal.WindowsIdentity]::GetCurrent().Name
icacls $root /inheritance:r /grant:r "${identity}:(OI)(CI)F" '*S-1-5-18:(OI)(CI)F' | Out-Null
if($LASTEXITCODE -ne 0){throw 'Cannot protect verification directory'}
function Secret { [Convert]::ToHexString([Security.Cryptography.RandomNumberGenerator]::GetBytes(32)) }
function Check($ok,[string]$message){if(-not $ok){throw $message}}
function Port { $listener=[Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback,0);$listener.Start();$p=$listener.LocalEndpoint.Port;$listener.Stop();$p }
function Basic([string]$user,[string]$password){ @{Authorization=('Basic '+[Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($user+':'+$password)))} }
function Call([string]$url,$headers,[string]$method='GET',$body=$null){
    $args=@{Uri=$url;Headers=$headers;Method=$method;SkipHttpErrorCheck=$true;TimeoutSec=10}
    if($null -ne $body){$args.ContentType='application/json';$args.Body=$body|ConvertTo-Json -Depth 60 -Compress}
    Invoke-WebRequest @args
}
function BrowserState([string]$baseURL,[string]$login,[string]$password,[string]$file) {
    $session=New-Object Microsoft.PowerShell.Commands.WebRequestSession
    $response=Invoke-WebRequest -Uri ($baseURL+'/login') -Method POST -ContentType 'application/json' -Body (@{user=$login;password=$password}|ConvertTo-Json -Compress) -Headers @{Origin=$baseURL} -WebSession $session -SkipHttpErrorCheck -TimeoutSec 10
    Check ($response.StatusCode -eq 200) 'Cannot establish the isolated browser session'
    $cookies=@(foreach($cookie in $session.Cookies.GetCookies([uri]$baseURL)){
        @{name=$cookie.Name;value=$cookie.Value;domain=$cookie.Domain;path=$cookie.Path;expires=if($cookie.Expires -eq [DateTime]::MinValue){-1}else{([DateTimeOffset]$cookie.Expires).ToUnixTimeSeconds()};httpOnly=$cookie.HttpOnly;secure=$cookie.Secure;sameSite='Lax'}
    })
    Check ($cookies.Count -gt 0) 'Browser login returned no session cookie'
    @{cookies=$cookies;origins=@()}|ConvertTo-Json -Depth 8|Set-Content -LiteralPath (Join-Path $root $file) -Encoding utf8
}
$panelPort=Port;$grafanaPort=Port;$panelToken=Secret;$panelPassword=Secret;$viewerPassword=Secret;$grafanaPassword=Secret
$panelBase='http://127.0.0.1:'+$panelPort;$grafanaBase='http://127.0.0.1:'+$grafanaPort
$panelHeaders=@{Authorization=('Bearer '+$panelToken)};$grafanaHeaders=Basic admin $grafanaPassword
$secretFile=Join-Path $root 'grafana-admin.secret'
Set-Content -LiteralPath $secretFile -Value $grafanaPassword -NoNewline -Encoding ascii
$container='mydb-grafana-verify-'+[guid]::NewGuid().ToString('N').Substring(0,10)
$panel=$null
try{
    $env:METRICSPANEL_TOKEN=$panelToken;$env:METRICSPANEL_ADMIN_USER='fixture-admin';$env:METRICSPANEL_ADMIN_PASSWORD=$panelPassword
    try{$panel=Start-Process -FilePath $MetricsPanelExe -WindowStyle Hidden -PassThru -ArgumentList @('serve','--auth','local','--listen',('127.0.0.1:'+$panelPort),'--db',(Join-Path $root 'panel.db')) -RedirectStandardOutput (Join-Path $root 'panel.out.log') -RedirectStandardError (Join-Path $root 'panel.err.log')}
    finally{Remove-Item Env:METRICSPANEL_TOKEN,Env:METRICSPANEL_ADMIN_USER,Env:METRICSPANEL_ADMIN_PASSWORD}
    $ready=$false
    for($i=0;$i -lt 80;$i++){try{if((Call ($panelBase+'/api/health') @{}).StatusCode -eq 200){$ready=$true;break}}catch{};Start-Sleep -Milliseconds 250}
    Check $ready 'MetricsPanel233 did not become ready'
    $viewer=Call ($panelBase+'/api/admin/users') (Basic 'fixture-admin' $panelPassword) 'POST' @{login='grafana-viewer';name='Grafana fixture';password=$viewerPassword;role='Viewer'}
    Check ($viewer.StatusCode -eq 200 -or $viewer.StatusCode -eq 201) 'Cannot create least-privilege Grafana viewer'
    Check ((Call ($panelBase+'/api/v1/connections/preview') (Basic 'grafana-viewer' $viewerPassword) 'POST' @{}).StatusCode -eq 403) 'Grafana viewer unexpectedly has monitoring management access'
    $token=(Get-Content -Raw -LiteralPath $MyDBTokenFile).Trim()
    $setup=@{id=[guid]::NewGuid().ToString();language='en';alerts=$false;target=@{name='mydb';kind='mydb';url=$MyDBMetricsURL;enabled=$true;interval_seconds=5;secure_settings=@{bearer_token=$token}}}
    $preview=Call ($panelBase+'/api/v1/connections/preview') $panelHeaders 'POST' @{setup=$setup}
    Check ($preview.StatusCode -eq 200) 'MyDB preview failed'
    $plan=$preview.Content|ConvertFrom-Json
    Check ((Call ($panelBase+'/api/v1/connections/apply') $panelHeaders 'POST' @{setup=$setup;plan_hash=$plan.plan_hash}).StatusCode -eq 200) 'MyDB setup failed'
    $id=& docker run --detach --name $container --read-only --cap-drop ALL --security-opt no-new-privileges --pids-limit 128 --memory 384m --cpus 1 --tmpfs '/var/lib/grafana:rw,size=128m,uid=472,gid=0' --tmpfs '/tmp:rw,size=32m' --mount ('type=bind,source='+$secretFile+',target=/run/secrets/admin,readonly') --publish ('127.0.0.1:'+ $grafanaPort+':3000') --env GF_SECURITY_ADMIN_PASSWORD__FILE=/run/secrets/admin --env GF_ANALYTICS_REPORTING_ENABLED=false --env GF_ANALYTICS_CHECK_FOR_UPDATES=false --env GF_PLUGINS_PREINSTALL_DISABLED=true --env GF_PLUGINS_PREINSTALL_AUTO_UPDATE=false $GrafanaImage
    Check ($LASTEXITCODE -eq 0) 'Grafana container failed to start'
    $ready=$false
    for($i=0;$i -lt 120;$i++){try{if((Call ($grafanaBase+'/api/health') @{}).StatusCode -eq 200){$ready=$true;break}}catch{};Start-Sleep -Milliseconds 500}
    Check $ready 'Grafana did not become ready'
    $source=Call ($grafanaBase+'/api/datasources') $grafanaHeaders 'POST' @{uid='mydb-fixture';name='Metrics233 read-only';type='prometheus';access='proxy';url=('http://host.docker.internal:'+$panelPort+'/prometheus');basicAuth=$true;basicAuthUser='grafana-viewer';secureJsonData=@{basicAuthPassword=$viewerPassword};jsonData=@{httpMethod='GET';prometheusType='Prometheus'}}
    Check ($source.StatusCode -eq 200) 'Grafana datasource creation failed'
    Check (-not $source.Content.Contains($viewerPassword)) 'Grafana datasource response exposed its password'
    Start-Sleep -Seconds 12
    $query=Call ($grafanaBase+'/api/ds/query') $grafanaHeaders 'POST' @{from='now-5m';to='now';queries=@(@{refId='A';datasource=@{type='prometheus';uid='mydb-fixture'};expr='mydb_up';instant=$true;range=$false;format='time_series';intervalMs=15000;maxDataPoints=100})}
    Check ($query.StatusCode -eq 200) ('Grafana datasource query failed: HTTP '+$query.StatusCode+' '+$query.Content)
    $data=$query.Content|ConvertFrom-Json
    Check ($data.results.A.frames.Count -eq 1 -and $data.results.A.frames[0].data.values[1][0] -eq 1) 'Grafana did not receive actual MyDB data'
    $dashboard=(Get-Content -Raw -LiteralPath $DashboardFile).Replace('${DS_PROMETHEUS}','mydb-fixture')|ConvertFrom-Json
    $saved=Call ($grafanaBase+'/api/dashboards/db') $grafanaHeaders 'POST' @{dashboard=$dashboard;overwrite=$false}
    Check ($saved.StatusCode -eq 200) 'Grafana dashboard import failed'
    $read=Call ($grafanaBase+'/api/dashboards/uid/mydb-native-performance') $grafanaHeaders
    Check ($read.StatusCode -eq 200) 'Grafana dashboard readback failed'
    $count=($read.Content|ConvertFrom-Json).dashboard.panels.Count
    Check ($count -eq 14) 'Grafana dashboard lost panels'
    $receipt=@{grafana_image=$GrafanaImage;viewer_isolation=$true;actual_datasource_query=$true;grafana_dashboard_panels=$count;browser_verified=$false}
    $receipt|ConvertTo-Json|Set-Content -LiteralPath (Join-Path $root 'receipt.json') -Encoding utf8
    $receipt|ConvertTo-Json -Compress
    if($BrowserHoldSeconds -gt 0){
        # Short-lived authentication state stays in this ACL-protected fixture.
        # Browser tools load it by path; no credential is printed or passed on a command line.
        BrowserState $panelBase 'fixture-admin' $panelPassword 'browser-panel-state.json'
        BrowserState $grafanaBase 'admin' $grafanaPassword 'browser-grafana-state.json'
        @{panel_url=($panelBase+'/#business');grafana_url=($grafanaBase+($saved.Content|ConvertFrom-Json).url);grafana_state=(Join-Path $root 'browser-grafana-state.json');panel_state=(Join-Path $root 'browser-panel-state.json')}|ConvertTo-Json|Set-Content -LiteralPath (Join-Path $root 'browser-ready.json') -Encoding utf8
        Write-Host ('Bounded browser fixture ready: '+$root)
        $deadline=(Get-Date).AddSeconds($BrowserHoldSeconds)
        while((Get-Date) -lt $deadline -and -not(Test-Path -LiteralPath (Join-Path $root 'browser.done'))){Start-Sleep -Milliseconds 250}
    }
}finally{
    & docker logs $container 2>&1 | Set-Content -LiteralPath (Join-Path $root 'grafana.log') -Encoding utf8
    & docker rm --force $container 2>&1 | Out-Null
    if($null -ne $panel -and -not $panel.HasExited){Stop-Process -Id $panel.Id -Force;$panel.WaitForExit(10000)|Out-Null}
    if(Test-Path -LiteralPath $secretFile){Remove-Item -LiteralPath $secretFile -Force}
    foreach($name in @('browser-panel-state.json','browser-grafana-state.json')){ $file=Join-Path $root $name;if(Test-Path -LiteralPath $file){Remove-Item -LiteralPath $file -Force} }
}
