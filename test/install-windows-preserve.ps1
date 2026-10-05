$ErrorActionPreference = 'Stop'
$scriptPath = Join-Path $PSScriptRoot '../scripts/install.ps1'
$scriptText = Get-Content $scriptPath -Raw
$testRoot = Join-Path ([System.IO.Path]::GetTempPath()) ('bililive-preserve-' + [guid]::NewGuid())
New-Item -ItemType Directory $testRoot | Out-Null
try {
    $ConfigFile = Join-Path $testRoot 'config.yml'
    $original = "rpc:`n  bind: :9090`nout_put_path: D:/Videos`napp_data_path: D:/Data`nsecurity:`n  enable_api_key: true`n  api_key: original-key`n"
    [System.IO.File]::WriteAllText($ConfigFile, $original)
    $before = [System.IO.File]::ReadAllBytes($ConfigFile)
    $Port = 8080
    $VideosDir = 'C:/default/Videos'
    $AppDataDir = 'C:/default/Data'
    $EnableApiKey = $true
    $ApiKey = 'new-key'
    $rewriteCalls = 0
    function Log($msg) {}
    function Warn($msg) {}
    function Ask-YesNo($prompt, $default) { return 'n' }
    function Set-ConfigValues($file) { $script:rewriteCalls++; Set-Content $file 'overwritten' }
    function Invoke-WebRequest { throw '保留配置时不应下载模板' }

    # 运行脚本中的真实配置安装段，隔离网络、系统任务和服务启动。
    $start = $scriptText.IndexOf('    if (Test-Path $ConfigFile) {', $scriptText.IndexOf('$ServiceStarted = $false'))
    $end = $scriptText.IndexOf('    # 注册开机自启', $start)
    if ($start -lt 0 -or $end -lt 0) { throw '找不到配置安装段' }
    Invoke-Expression $scriptText.Substring($start, $end - $start)
    $after = [System.IO.File]::ReadAllBytes($ConfigFile)
    if ([Convert]::ToBase64String($before) -ne [Convert]::ToBase64String($after)) { throw '旧配置字节被改写' }
    if ($rewriteCalls -ne 0) { throw '保留旧配置却调用了重写逻辑' }
    if ($Port -ne 9090 -or $VideosDir -ne 'D:/Videos' -or $AppDataDir -ne 'D:/Data') { throw 'doctor 和展示没有沿用旧目录、端口' }
    if ($EnableApiKey) { throw '保留配置时不应展示尚未写入的新 Key' }
    Write-Host 'PASS: Windows 更新逐字节保留配置，沿用旧端口和目录'
} finally {
    Remove-Item $testRoot -Recurse -Force
}
