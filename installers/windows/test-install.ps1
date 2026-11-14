# Run only on an ephemeral Windows CI runner.
# Never execute this acceptance test on the owner's machine.
param(
  [Parameter(Mandatory = $true)][string]$Installer,
  [Parameter(Mandatory = $true)][string]$SandboxDir
)
$ErrorActionPreference = 'Stop'

function Assert-True([bool]$condition, [string]$message) {
  if (-not $condition) { throw $message }
}

$installerPath = (Resolve-Path -LiteralPath $Installer).Path
$testRoot = [IO.Path]::GetFullPath($SandboxDir)
$installDir = Join-Path $testRoot 'installed'
$installLog = Join-Path $testRoot 'install.log'
$uninstallLog = Join-Path $testRoot 'uninstall.log'
$desktop = [Environment]::GetFolderPath('Desktop')
$programs = [Environment]::GetFolderPath('Programs')
$startMenuLink = Join-Path $programs 'Dirhaven\Dirhaven Setup.lnk'
$desktopLink = Join-Path $desktop 'Dirhaven Setup.lnk'

Assert-True ($env:CI -eq 'true' -and $env:GITHUB_ACTIONS -eq 'true') 'Refusing to run outside a GitHub Actions ephemeral runner'
Assert-True (Test-Path -LiteralPath $installerPath -PathType Leaf) 'Installer missing'
Assert-True (-not (Test-Path -LiteralPath $installDir)) 'Test installation destination already exists'
Assert-True (-not (Test-Path -LiteralPath $startMenuLink)) 'Start Menu shortcut already exists - refusing to alter'
Assert-True (-not (Test-Path -LiteralPath $desktopLink)) 'Desktop shortcut already exists - refusing to alter'

New-Item -Path $testRoot -ItemType Directory -Force | Out-Null
Write-Output "Testing install into isolated CI temp directory"

# The post-install wizard is intentionally suppressed in silent mode.
$installArgs = @('/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', '/SP-', '/TASKS=desktopicon', "/DIR=$installDir", "/LOG=$installLog")
$installProcess = Start-Process -FilePath $installerPath -ArgumentList $installArgs -Wait -PassThru
Assert-True ($installProcess.ExitCode -eq 0) "Installer failed: exit code $($installProcess.ExitCode)"
$exe = Join-Path $installDir 'dirhaven.exe'
Assert-True (Test-Path -LiteralPath $exe -PathType Leaf) 'Installed binary missing'
Assert-True (Test-Path -LiteralPath (Join-Path $installDir 'unins000.exe') -PathType Leaf) 'Uninstaller missing'
Assert-True (Test-Path -LiteralPath (Join-Path $installDir 'LICENSE')) 'Project license missing'
Assert-True (Test-Path -LiteralPath (Join-Path $installDir 'third_party\licenses') -PathType Container) 'Third-party notices missing'
Assert-True (Test-Path -LiteralPath $startMenuLink -PathType Leaf) 'Start Menu shortcut missing'
Assert-True (Test-Path -LiteralPath $desktopLink -PathType Leaf) 'Desktop shortcut missing'

# Verify the command resolves without opening a browser, tunnel or MCP service.
$versionOutput = & $exe version
Assert-True ($LASTEXITCODE -eq 0) "Installed binary 'version' failed"
Assert-True ($versionOutput -match 'dirhaven 0.2.0-dev \(windows/amd64\)') "Unexpected installed version: $versionOutput"
Write-Output "Installed version: $versionOutput"

$shell = New-Object -ComObject WScript.Shell
$startShortcut = $shell.CreateShortcut($startMenuLink)
$desktopShortcut = $shell.CreateShortcut($desktopLink)
Assert-True ($startShortcut.TargetPath -eq $exe) 'Start Menu target incorrect'
Assert-True ($startShortcut.Arguments -eq 'setup') 'Start Menu arguments incorrect'
Assert-True ($desktopShortcut.TargetPath -eq $exe) 'Desktop target incorrect'
Assert-True ($desktopShortcut.Arguments -eq 'setup') 'Desktop arguments incorrect'

Write-Output 'Running silent uninstall'
$uninstallExe = Join-Path $installDir 'unins000.exe'
$uninstallArgs = @('/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', "/LOG=$uninstallLog")
$uninstallProcess = Start-Process -FilePath $uninstallExe -ArgumentList $uninstallArgs -Wait -PassThru
Assert-True ($uninstallProcess.ExitCode -eq 0) "Uninstaller failed: exit code $($uninstallProcess.ExitCode)"

# Inno Setup can defer file removal briefly until its child uninstaller exits.
for ($attempt = 0; $attempt -lt 20; $attempt++) {
  if (-not (Test-Path -LiteralPath $exe) -and
      -not (Test-Path -LiteralPath $startMenuLink) -and
      -not (Test-Path -LiteralPath $desktopLink)) { break }
  Start-Sleep -Milliseconds 500
}
Assert-True (-not (Test-Path -LiteralPath $exe)) 'Binary remained after uninstall'
Assert-True (-not (Test-Path -LiteralPath $startMenuLink)) 'Start Menu shortcut remained after uninstall'
Assert-True (-not (Test-Path -LiteralPath $desktopLink)) 'Desktop shortcut remained after uninstall'
Assert-True (-not (Test-Path -LiteralPath (Join-Path $installDir 'LICENSE'))) 'License file remained after uninstall'
Write-Output 'PASS: isolated install, version, shortcuts, notices, uninstall and cleanup'
