# ============================================================
# install.ps1 — bililive-go Windows 一键安装脚本（统一安装流程）
#
# 流程：检测系统 → 选择源（GitHub/自建源）→ 选择安装方式 → 检测原数据
#       → 检测工具 → 确认目录端口 → 拉取启动 → doctor 检测
#
# 交互模式：
#   irm https://<你的源站>/install.ps1 | iex
#   irm https://raw.githubusercontent.com/xuyuanzhang1122/bililive-go-UI/main/scripts/install.ps1 | iex
#
# 参数模式（先下载再执行）：
#   .\install.ps1 -Dir C:\bililive-go -Port 8080 -Source github -Yes
# ============================================================

[CmdletBinding()]
param(
    [string]$Dir = "",
    [string]$VideosDir = "",
    [int]$Port = 0,
    [string]$Source = "",
    [string]$Version = "",
    [switch]$EnableApiKey,
    [string]$ApiKey = "",
    [switch]$Yes,
    [ValidateSet("", "binary", "docker")]
    [string]$Mode = ""
)

$ErrorActionPreference = "Stop"
$Repo = "xuyuanzhang1122/bililive-go-UI"
$RawBase = "https://raw.githubusercontent.com/$Repo/main"
# 由源站 /install.ps1 动态注入为该源站地址；直接从 GitHub raw 获取时为空
$DefaultMirror = ""

function Log($msg)  { Write-Host "-> $msg" -ForegroundColor Cyan }
function Ok($msg)   { Write-Host "[OK] $msg" -ForegroundColor Green }
function Warn($msg) { Write-Host "[!] $msg" -ForegroundColor Yellow }
function Err($msg)  { Write-Host "[X] $msg" -ForegroundColor Red }

function Read-Default([string]$Prompt, [string]$Default) {
    if ($Yes) {
        Write-Host "$Prompt [$Default]: $Default (auto)"
        return $Default
    }
    $answer = Read-Host "$Prompt [$Default]"
    if ([string]::IsNullOrWhiteSpace($answer)) { return $Default }
    return $answer
}

function Ask-YesNo([string]$Prompt, [string]$Default = "n") {
    if ($Yes) { return $Default }
    $hint = if ($Default -eq "y") { "Y/n" } else { "y/N" }
    $answer = Read-Host "$Prompt [$hint]"
    switch -Regex ($answer.ToLower()) {
        "^(y|yes)$" { return "y" }
        "^(n|no)$"  { return "n" }
        default     { return $Default }
    }
}

function New-RandomApiKey {
    $bytes = New-Object byte[] 32
    [System.Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($bytes)
    return ($bytes | ForEach-Object { $_.ToString("x2") }) -join ""
}

# ============================================================
# 第 1 步：检测系统
# ============================================================
Log "[1/7] 检测系统环境"
$arch = switch ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString().ToLower()) {
    "x64"   { "amd64" }
    "arm64" { "arm64" }
    default { Err "不支持的 CPU 架构"; exit 1 }
}
Ok "系统: windows / $arch"

# ============================================================
# 第 2 步：选择安装源
# ============================================================
Write-Host ""
Log "[2/7] 选择安装源"
$MirrorUrl = ""
if ([string]::IsNullOrWhiteSpace($Source)) {
    if (-not [string]::IsNullOrWhiteSpace($DefaultMirror)) {
        Write-Host "  [1] 自建源 $DefaultMirror（推荐，含工具分发）"
        Write-Host "  [2] GitHub 官方"
        $choice = Read-Default "选择源 (1=自建源 2=GitHub)" "1"
        $Source = if ($choice -eq "2") { "github" } else { $DefaultMirror }
    } else {
        Write-Host "  [1] GitHub 官方"
        Write-Host "  [2] 自建源（输入地址）"
        $choice = Read-Default "选择源 (1=GitHub 2=自建源)" "1"
        if ($choice -eq "2") {
            $Source = Read-Default "自建源地址（如 https://update.example.com）" ""
            if ([string]::IsNullOrWhiteSpace($Source)) { Err "未输入源地址"; exit 1 }
        } else {
            $Source = "github"
        }
    }
}
if ($Source -ne "github") {
    $MirrorUrl = $Source.TrimEnd("/")
    Log "校验自建源: $MirrorUrl"
    try {
        Invoke-RestMethod -Uri "$MirrorUrl/health" -TimeoutSec 5 | Out-Null
        Ok "自建源可用"
    } catch {
        Warn "自建源无响应，回退到 GitHub"
        $MirrorUrl = ""
        $Source = "github"
    }
}
if ($Source -eq "github") { Ok "使用 GitHub 官方源" }

$Catalog = $null
if ($MirrorUrl) {
    try {
        $Catalog = Invoke-RestMethod -Uri "$MirrorUrl/api/v1/catalog" -TimeoutSec 10
    } catch {
        Warn "无法获取自建源 catalog，二进制将回退 GitHub 下载"
    }
}

# ============================================================
# 第 3 步：选择安装方式
# ============================================================
Write-Host ""
Log "[3/7] 选择安装方式"
if ([string]::IsNullOrWhiteSpace($Mode)) {
    Write-Host "  [1] 二进制 Release（默认）"
    Write-Host "  [2] Docker 容器（需 Docker Desktop）"
    $choice = Read-Default "选择安装方式 (1=二进制 2=Docker)" "1"
    $Mode = if ($choice -eq "2") { "docker" } else { "binary" }
}
Ok "安装方式: $Mode"
if ($Mode -eq "docker") {
    if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
        Err "未检测到 Docker。请先安装 Docker Desktop: https://docs.docker.com/desktop/install/windows-install/"
        exit 1
    }
    try { docker info 2>&1 | Out-Null } catch {
        Err "Docker 守护进程未运行，请先启动 Docker Desktop"
        exit 1
    }
}

# ============================================================
# 第 4 步：确认目录与端口（含原版本数据检测）
# ============================================================
Write-Host ""
Log "[4/7] 确认安装目录、视频目录与端口"

if ([string]::IsNullOrWhiteSpace($Dir)) {
    $Dir = Read-Default "安装目录（程序/配置/数据放这里）" "$env:USERPROFILE\bililive-go"
}
$Dir = [System.IO.Path]::GetFullPath($Dir)

if ([string]::IsNullOrWhiteSpace($VideosDir)) {
    $foundOld = ""
    foreach ($candidate in @("$Dir\Videos", "$env:USERPROFILE\bililive-go\Videos", "$env:USERPROFILE\bililive\Videos")) {
        if (Test-Path $candidate) {
            $hasVideo = Get-ChildItem -Path $candidate -Recurse -Depth 3 -Include *.flv, *.ts, *.mp4 -ErrorAction SilentlyContinue | Select-Object -First 1
            if ($hasVideo) { $foundOld = $candidate; break }
        }
    }
    if ($foundOld) {
        Warn "检测到原版本视频数据: $foundOld"
        if ((Ask-YesNo "复用该视频目录？（新录制和旧视频都放这里）" "y") -eq "y") {
            $VideosDir = $foundOld
        }
    }
    if ([string]::IsNullOrWhiteSpace($VideosDir)) {
        $VideosDir = Read-Default "视频目录（可指向原版本数据位置）" "$Dir\Videos"
    }
}
$VideosDir = [System.IO.Path]::GetFullPath($VideosDir)

if ($Port -le 0) {
    $Port = [int](Read-Default "Web UI 端口" "8080")
}
if ($Port -lt 1 -or $Port -gt 65535) { Err "端口非法: $Port"; exit 1 }

$portInUse = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue
if ($portInUse) {
    Warn "端口 $Port 已被占用"
    if ((Ask-YesNo "继续使用此端口？" "n") -ne "y") { Err "已取消"; exit 1 }
}

if (-not $EnableApiKey) {
    if ((Ask-YesNo "启用 API Key 鉴权？（公网部署建议开启）" "n") -eq "y") { $EnableApiKey = $true }
}
if ($EnableApiKey -and [string]::IsNullOrWhiteSpace($ApiKey)) {
    $ApiKey = New-RandomApiKey
}

New-Item -ItemType Directory -Force -Path $Dir, $VideosDir, "$Dir\Data" | Out-Null
Ok "安装目录: $Dir"
Ok "视频目录: $VideosDir"
Ok "端口: $Port"

# ============================================================
# 第 5 步：检测/拉取依赖工具
# ============================================================
Write-Host ""
Log "[5/7] 检测依赖工具"
$ToolsDir = "$Dir\tools"
$FfmpegPath = ""
$HeadlessPath = ""

$ffmpegCmd = Get-Command ffmpeg -ErrorAction SilentlyContinue
if ($ffmpegCmd) { $FfmpegPath = $ffmpegCmd.Source }

function Get-MirrorTool([string]$Name) {
    if (-not $Catalog -or -not $Catalog.tools) { return "" }
    $tool = $Catalog.tools | Where-Object { $_.name -eq $Name -and $_.os -eq "windows" -and $_.arch -eq $arch -and $_.url } | Select-Object -First 1
    if (-not $tool) { return "" }
    New-Item -ItemType Directory -Force -Path $ToolsDir | Out-Null
    $fname = Split-Path $tool.url -Leaf
    $dest = Join-Path $ToolsDir $fname
    Log "从自建源拉取 ${Name}: $MirrorUrl$($tool.url)"
    try {
        Invoke-WebRequest -Uri "$MirrorUrl$($tool.url)" -OutFile $dest -UseBasicParsing
    } catch {
        Warn "拉取 $Name 失败: $_"
        return ""
    }
    if ($fname -match "\.zip$") {
        Expand-Archive -Path $dest -DestinationPath $ToolsDir -Force
        Remove-Item $dest -Force
        $exe = Get-ChildItem -Path $ToolsDir -Recurse -Include "$Name*.exe" | Select-Object -First 1
        if ($exe) { return $exe.FullName }
        return ""
    }
    return $dest
}

if (-not $FfmpegPath -and $MirrorUrl) { $FfmpegPath = Get-MirrorTool "ffmpeg" }
if ($FfmpegPath) {
    Ok "ffmpeg: $FfmpegPath"
} elseif ($Mode -eq "docker") {
    Ok "ffmpeg: 镜像内置，跳过"
} else {
    Warn "未找到 ffmpeg：录制、缩略图、HLS 转封装将不可用"
    Write-Host "  安装方法: winget install ffmpeg 或 https://www.gyan.dev/ffmpeg/builds/"
}

foreach ($candidate in @(
    "$env:ProgramFiles\Google\Chrome\Application\chrome.exe",
    "${env:ProgramFiles(x86)}\Google\Chrome\Application\chrome.exe",
    "$env:ProgramFiles\Chromium\Application\chrome.exe",
    "$env:LOCALAPPDATA\Chromium\Application\chrome.exe"
)) {
    if (Test-Path $candidate) { $HeadlessPath = $candidate; break }
}
if (-not $HeadlessPath -and $MirrorUrl) { $HeadlessPath = Get-MirrorTool "headless-browser" }
if ($HeadlessPath) {
    Ok "无头浏览器: $HeadlessPath"
} else {
    Warn "未找到无头浏览器（可选）：非标准短链 JS 跳转解析会降级"
}

# ============================================================
# 第 6 步：下载并部署
# ============================================================
Write-Host ""
Log "[6/7] 下载并部署"

function Set-ConfigValues([string]$ConfigFile) {
    if (-not (Test-Path $ConfigFile)) { return }
    $lines = Get-Content $ConfigFile
    $inSecurity = $false
    $inHeadless = $false
    $out = foreach ($line in $lines) {
        if ($line -match "^security:\s*$") { $inSecurity = $true; $inHeadless = $false; $line; continue }
        if ($line -match "^headless_browser:\s*$") { $inHeadless = $true; $inSecurity = $false; $line; continue }
        if ($line -match "^[A-Za-z]" -and $line -notmatch "^(security|headless_browser):") { $inSecurity = $false; $inHeadless = $false }

        if ($line -match "^out_put_path:") { "out_put_path: $($VideosDir -replace '\\', '/')"; continue }
        if ($line -match "^app_data_path:") { "app_data_path: $(($Dir + '\Data') -replace '\\', '/')"; continue }
        if ($line -match "^ffmpeg_path:" -and $FfmpegPath) { "ffmpeg_path: `"$($FfmpegPath -replace '\\', '/')`""; continue }
        if ($line -match "^\s*bind:") { ($line -replace "bind:.*", "bind: :$Port"); continue }
        if ($inHeadless -and $line -match "^\s*path:" -and $HeadlessPath) { ($line -replace "path:.*", "path: `"$($HeadlessPath -replace '\\', '/')`""); continue }
        if ($inSecurity -and $EnableApiKey -and $line -match "^\s*enable_api_key:") { ($line -replace "enable_api_key:.*", "enable_api_key: true"); continue }
        if ($inSecurity -and $EnableApiKey -and $line -match "^\s*api_key:") { ($line -replace "api_key:.*", "api_key: `"$ApiKey`""); continue }
        $line
    }
    Set-Content -Path $ConfigFile -Value $out -Encoding UTF8
}

$ServiceStarted = $false
if ($Mode -eq "binary") {
    if ([string]::IsNullOrWhiteSpace($Version)) { $Version = "latest" }
    $asset = "bililive-windows-$arch.zip"
    $downloadUrl = ""
    if ($Catalog -and $Version -eq "latest") {
        $rel = $Catalog.releases | Where-Object { $_.name -eq $asset } | Select-Object -First 1
        if ($rel -and $rel.url) {
            $Version = $rel.version
            $downloadUrl = if ($rel.url -match "^http") { $rel.url } else { "$MirrorUrl$($rel.url)" }
            Log "自建源版本: $Version"
        }
    }
    if (-not $downloadUrl) {
        if ($Version -eq "latest") {
            Log "查询 GitHub 最新版本…"
            $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest"
            $Version = $release.tag_name
            Log "最新版本: $Version"
        }
        $downloadUrl = "https://github.com/$Repo/releases/download/$Version/$asset"
    }

    Log "下载: $downloadUrl"
    $tmpZip = Join-Path $env:TEMP $asset
    Invoke-WebRequest -Uri $downloadUrl -OutFile $tmpZip -UseBasicParsing
    $tmpExtract = Join-Path $env:TEMP "bililive-extract-$(Get-Random)"
    Expand-Archive -Path $tmpZip -DestinationPath $tmpExtract -Force
    $exe = Get-ChildItem -Path $tmpExtract -Recurse -Include "bililive*.exe" | Select-Object -First 1
    if (-not $exe) { Err "压缩包内未找到 exe"; exit 1 }
    $TargetBin = "$Dir\bililive-go.exe"
    Copy-Item $exe.FullName $TargetBin -Force
    Remove-Item $tmpZip, $tmpExtract -Recurse -Force -ErrorAction SilentlyContinue
    Ok "已安装到 $TargetBin"

    $ConfigFile = "$Dir\config.yml"
    if (Test-Path $ConfigFile) {
        Warn "配置文件已存在: $ConfigFile"
        if ((Ask-YesNo "用最新模板覆盖？（旧文件会备份为 .bak）" "n") -eq "y") {
            Copy-Item $ConfigFile "$ConfigFile.bak.$(Get-Date -Format yyyyMMddHHmmss)" -Force
            Invoke-WebRequest -Uri "$RawBase/config.yml" -OutFile $ConfigFile -UseBasicParsing
        }
    } else {
        Log "下载配置模板 → $ConfigFile"
        Invoke-WebRequest -Uri "$RawBase/config.yml" -OutFile $ConfigFile -UseBasicParsing
    }
    Set-ConfigValues $ConfigFile

    # 注册开机自启（计划任务，登录即启动）并立即启动
    $taskName = "bililive-go"
    Log "注册计划任务: $taskName（登录自启）"
    try {
        $action = New-ScheduledTaskAction -Execute $TargetBin -Argument "-c `"$ConfigFile`"" -WorkingDirectory $Dir
        $trigger = New-ScheduledTaskTrigger -AtLogOn
        Register-ScheduledTask -TaskName $taskName -Action $action -Trigger $trigger -Force | Out-Null
        Ok "计划任务已注册"
    } catch {
        Warn "计划任务注册失败（可能需要管理员权限）: $_"
    }
    Log "启动 bililive-go …"
    Start-Process -FilePath $TargetBin -ArgumentList "-c", $ConfigFile -WorkingDirectory $Dir -WindowStyle Hidden
    $ServiceStarted = $true
} else {
    $ContainerName = "bililive-go"
    if ([string]::IsNullOrWhiteSpace($Version)) { $Version = "latest" }
    $existing = docker ps -a --format '{{.Names}}' | Where-Object { $_ -eq $ContainerName }
    if ($existing) {
        Warn "已存在容器 '$ContainerName'"
        if ((Ask-YesNo "删除旧容器并重建？（数据保留在 $Dir）" "y") -ne "y") { Err "已取消"; exit 1 }
        docker rm -f $ContainerName | Out-Null
    }
    $ConfigFile = "$Dir\config.docker.yml"
    if (-not (Test-Path $ConfigFile)) {
        Log "下载配置模板 → $ConfigFile"
        Invoke-WebRequest -Uri "$RawBase/config.docker.yml" -OutFile $ConfigFile -UseBasicParsing
    }
    if ($EnableApiKey) { Set-ConfigValues $ConfigFile }

    Log "拉取镜像 xuniubi/bililive-go:$Version …"
    docker pull "xuniubi/bililive-go:$Version"
    Log "启动容器 …"
    docker run -d --name $ContainerName --restart unless-stopped `
        -p "${Port}:8080" `
        -v "${VideosDir}:/srv/bililive" `
        -v "${Dir}\Data:/var/lib/bililive" `
        -v "${ConfigFile}:/etc/bililive-go/config.yml" `
        "xuniubi/bililive-go:$Version" | Out-Null
    $ServiceStarted = $true
}

# ============================================================
# 第 7 步：doctor 检测
# ============================================================
Write-Host ""
Log "[7/7] doctor 检测"

$url = "http://127.0.0.1:$Port/api/auth-status"
$ready = $false
for ($i = 0; $i -lt 30; $i++) {
    try {
        Invoke-RestMethod -Uri $url -TimeoutSec 2 | Out-Null
        $ready = $true; break
    } catch { Start-Sleep -Seconds 1 }
}

function Doctor-Check([string]$Name, [bool]$IsOk, [string]$Hint) {
    if ($IsOk) { Ok $Name } else { Warn "$Name — $Hint" }
}

Doctor-Check "服务响应 ($url)" $ready "未在 30s 内就绪，请检查日志"
if ($Mode -eq "binary") {
    Doctor-Check "配置文件 ($ConfigFile)" (Test-Path $ConfigFile) "缺失"
    Doctor-Check "ffmpeg" ([bool]$FfmpegPath) "未找到，录制功能不可用"
}
Doctor-Check "视频目录可写" (Test-Path $VideosDir) "目录不存在"
Doctor-Check "无头浏览器（可选）" ([bool]$HeadlessPath) "短链 JS 跳转解析降级"

if ($MirrorUrl) {
    $report = @{
        os = "windows"; arch = $arch; install_mode = $Mode; port = $Port
        paths = @{ output_path = $VideosDir }
        tools = @(
            @{ id = "ffmpeg"; path = "$FfmpegPath"; ok = [bool]$FfmpegPath },
            @{ id = "headless-browser"; path = "$HeadlessPath"; ok = [bool]$HeadlessPath }
        )
    } | ConvertTo-Json -Depth 5
    try {
        Invoke-RestMethod -Uri "$MirrorUrl/api/v1/doctor" -Method Post -Body $report -ContentType "application/json" -TimeoutSec 5 | Out-Null
    } catch { }
}

# ============================================================
# 完成
# ============================================================
Write-Host ""
if ($ready) { Ok "bililive-go 已启动并就绪" } else { Warn "服务未就绪，请检查日志" }

Write-Host ""
Write-Host "=== 安装完成 ===" -ForegroundColor Green
Write-Host ""
Write-Host "  Web UI       : http://<服务器 IP>:$Port"
Write-Host "  本机访问     : http://127.0.0.1:$Port"
Write-Host "  配置文件     : $ConfigFile"
Write-Host "  数据目录     : $Dir\Data"
Write-Host "  视频目录     : $VideosDir"
if ($EnableApiKey) {
    Write-Host ""
    Write-Host "  API Key（请妥善保存）: $ApiKey" -ForegroundColor Yellow
    Write-Host "  iOS App / 外部客户端请把上面这串粘贴进设置页。"
}
Write-Host ""
if ($Mode -eq "docker") {
    Write-Host "  常用命令     :"
    Write-Host "    docker logs -f bililive-go     # 看日志"
    Write-Host "    docker restart bililive-go     # 重启"
} else {
    Write-Host "  服务管理     :"
    Write-Host "    计划任务 'bililive-go' 登录自启；taskschd.msc 可管理"
    Write-Host "    停止: Stop-Process -Name bililive-go"
}
Write-Host ""
