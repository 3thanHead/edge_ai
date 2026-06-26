# Root launcher for the iot_ai control CLI (Windows).
# Usage:  .\iot.ps1 <command> [args]   e.g.  .\iot.ps1 install-node   .\iot.ps1 cluster
$ErrorActionPreference = "Stop"
$py = (Get-Command python -ErrorAction SilentlyContinue) ?? (Get-Command python3 -ErrorAction SilentlyContinue)
if (-not $py) {
    Write-Error "Python 3 is required but was not found. Install it (e.g. winget install Python.Python.3) and retry."
    exit 1
}
& $py.Source "$PSScriptRoot\tools\iotctl\iotctl.py" @args
exit $LASTEXITCODE
