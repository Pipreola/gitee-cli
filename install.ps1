#Requires -Version 5.1
<#
.SYNOPSIS
    gitee-cli Windows 原生安装脚本 (PowerShell)

.DESCRIPTION
    自动检测系统架构 (x64/arm64)，从 GitHub Releases 下载对应的 Windows 版本，
    解压并安装到 $env:LOCALAPPDATA\Programs\gitee，并将其加入用户 PATH 环境变量。

.PARAMETER Version
    指定要安装的版本 (形如 v1.0.0)。默认安装最新版本。

.PARAMETER InstallDir
    指定安装目录。默认 $env:LOCALAPPDATA\Programs\gitee。

.EXAMPLE
    irm https://raw.githubusercontent.com/Pipreola/gitee-cli/main/install.ps1 | iex

.EXAMPLE
    # 安装到自定义目录、指定版本
    & ([scriptblock]::Create((irm https://raw.githubusercontent.com/Pipreola/gitee-cli/main/install.ps1))) -Version v1.0.0
#>
[CmdletBinding()]
param(
    [string]$Version = "",
    [string]$InstallDir = ""
)

$ErrorActionPreference = "Stop"

# 配置
$RepoOwner   = "Pipreola"
$RepoName    = "gitee-cli"
$BinaryName  = "gitee.exe"

if ([string]::IsNullOrWhiteSpace($InstallDir)) {
    $InstallDir = Join-Path $env:LOCALAPPDATA "Programs\gitee"
}

# 日志函数（带颜色）
function Write-Info    { param([string]$Message) Write-Host "[信息] " -ForegroundColor Blue   -NoNewline; Write-Host $Message }
function Write-Success { param([string]$Message) Write-Host "[成功] " -ForegroundColor Green  -NoNewline; Write-Host $Message }
function Write-Warn    { param([string]$Message) Write-Host "[警告] " -ForegroundColor Yellow -NoNewline; Write-Host $Message }
function Write-Err     { param([string]$Message) Write-Host "[错误] " -ForegroundColor Red    -NoNewline; Write-Host $Message }

# 检测 CPU 架构，返回 GoReleaser 产物命名所用的架构标识
function Get-Arch {
    $arch = $env:PROCESSOR_ARCHITECTURE
    switch ($arch) {
        "AMD64" { return "x86_64" }
        "ARM64" { return "arm64" }
        "x86"   {
            # 32 位进程运行在 64 位系统上时，通过 PROCESSOR_ARCHITEW6432 还原真实架构
            if ($env:PROCESSOR_ARCHITEW6432 -eq "AMD64") { return "x86_64" }
            if ($env:PROCESSOR_ARCHITEW6432 -eq "ARM64") { return "arm64" }
            throw "不支持 32 位 (x86) 架构，gitee-cli 仅提供 64 位 Windows 版本"
        }
        default { throw "无法识别的 CPU 架构: $arch" }
    }
}

# 获取最新版本号 (tag)
function Get-LatestVersion {
    $apiUrl = "https://api.github.com/repos/$RepoOwner/$RepoName/releases/latest"
    try {
        $headers = @{ "User-Agent" = "gitee-cli-installer" }
        $release = Invoke-RestMethod -Uri $apiUrl -Headers $headers -UseBasicParsing
        if ([string]::IsNullOrWhiteSpace($release.tag_name)) {
            throw "响应中缺少 tag_name 字段"
        }
        return $release.tag_name
    } catch {
        throw "无法获取最新版本号: $($_.Exception.Message)"
    }
}

# 构建下载 URL
# 产物名遵循 GoReleaser: gitee-cli_{version}_Windows_{arch}.zip (version 不含前导 v)
function Get-DownloadUrl {
    param([string]$Tag, [string]$Arch)
    $version  = $Tag.TrimStart("v")
    $filename = "${RepoName}_${version}_Windows_${Arch}.zip"
    return "https://github.com/$RepoOwner/$RepoName/releases/download/$Tag/$filename"
}

# 将目录加入用户 PATH（持久化到注册表 + 当前会话）
function Add-ToUserPath {
    param([string]$Dir)
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ([string]::IsNullOrEmpty($userPath)) { $userPath = "" }

    $existing = $userPath.Split(';', [StringSplitOptions]::RemoveEmptyEntries)
    foreach ($p in $existing) {
        if ($p.TrimEnd('\') -ieq $Dir.TrimEnd('\')) {
            Write-Info "安装目录已在 PATH 中，跳过"
            return $false
        }
    }

    $newPath = if ([string]::IsNullOrEmpty($userPath)) { $Dir } else { "$($userPath.TrimEnd(';'));$Dir" }
    [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
    # 同步更新当前会话的 PATH，使本次安装后立即可用
    $env:Path = "$env:Path;$Dir"
    Write-Success "已将 $Dir 添加到用户 PATH"
    return $true
}

# 主流程
function Install-GiteeCli {
    Write-Host ""
    Write-Host "+======================================+" -ForegroundColor Cyan
    Write-Host "|   gitee-cli Windows 安装脚本         |" -ForegroundColor Cyan
    Write-Host "+======================================+" -ForegroundColor Cyan
    Write-Host ""

    # 1. 检测架构
    $arch = Get-Arch
    Write-Info "操作系统: Windows"
    Write-Info "架构: $arch"

    # 2. 解析版本
    $tag = $Version
    if ([string]::IsNullOrWhiteSpace($tag)) {
        Write-Info "获取最新版本..."
        $tag = Get-LatestVersion
    }
    Write-Info "目标版本: $tag"

    # 3. 构建下载 URL
    $url = Get-DownloadUrl -Tag $tag -Arch $arch

    # 4. 下载到临时目录
    $tmpDir  = Join-Path ([System.IO.Path]::GetTempPath()) ("gitee-cli-" + [System.Guid]::NewGuid().ToString("N"))
    New-Item -ItemType Directory -Path $tmpDir -Force | Out-Null
    $zipPath = Join-Path $tmpDir "gitee-cli.zip"

    try {
        Write-Info "下载中: $url"
        try {
            Invoke-WebRequest -Uri $url -OutFile $zipPath -UseBasicParsing
        } catch {
            throw "下载失败 ($($_.Exception.Message))。请确认版本 $tag 存在对应的 Windows ($arch) 产物。"
        }

        # 5. 解压
        Write-Info "解压中..."
        $extractDir = Join-Path $tmpDir "extracted"
        New-Item -ItemType Directory -Path $extractDir -Force | Out-Null
        Expand-Archive -Path $zipPath -DestinationPath $extractDir -Force

        # 6. 定位 gitee.exe
        $binary = Get-ChildItem -Path $extractDir -Filter $BinaryName -Recurse -File | Select-Object -First 1
        if ($null -eq $binary) {
            throw "压缩包中未找到 $BinaryName"
        }

        # 7. 安装到目标目录
        if (-not (Test-Path $InstallDir)) {
            New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
        }
        $targetPath = Join-Path $InstallDir $BinaryName
        Write-Info "安装到 $InstallDir ..."
        Copy-Item -Path $binary.FullName -Destination $targetPath -Force

        # 8. 加入 PATH
        Add-ToUserPath -Dir $InstallDir | Out-Null

        Write-Host ""
        Write-Success "gitee-cli 安装成功！"

        # 9. 验证
        Write-Info "验证安装..."
        try {
            $versionOutput = & $targetPath version 2>&1 | Out-String
            Write-Host ""
            Write-Host $versionOutput.Trim()
        } catch {
            Write-Warn "已安装，但执行版本检查时出现问题: $($_.Exception.Message)"
        }

        Write-Host ""
        Write-Info "开始使用:"
        Write-Host "  1. 重新打开终端，或在当前会话直接运行 gitee（PATH 已更新）"
        Write-Host "  2. 登录 Gitee: gitee auth login"
        Write-Host "  3. 查看帮助:   gitee --help"
        Write-Host ""
        Write-Info "文档: https://github.com/$RepoOwner/$RepoName"
        Write-Host ""
    } finally {
        # 清理临时文件
        if (Test-Path $tmpDir) {
            Remove-Item -Path $tmpDir -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
}

try {
    Install-GiteeCli
} catch {
    Write-Err $_.Exception.Message
    exit 1
}
