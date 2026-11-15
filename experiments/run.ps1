# One command: resimulate the grid, then write the table and the figures.
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root
go run ./experiments/sweep -out experiments/out @args
$python = Get-Command python -ErrorAction SilentlyContinue
if (-not $python) {
    $python = Get-Command py -ErrorAction SilentlyContinue
}
if (-not $python) {
    throw "python was not found on PATH"
}
& $python.Source experiments/plot.py experiments/out
