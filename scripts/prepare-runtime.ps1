param(
    [string]$ArchivePath = (Join-Path $PSScriptRoot '..\internal\capture\bundled\qq-runtime.zip')
)
$ErrorActionPreference = 'Stop'
$expected = 'E0B4B731DF9E054F130D4D6D037EB09DB67E433B214DD495D14A0312C0111C17'
$target = [IO.Path]::GetFullPath($ArchivePath)
if (Test-Path -LiteralPath $target) {
    if ((Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash -ne $expected) {
        throw 'Existing runtime archive does not match the pinned SHA256; it was not replaced.'
    }
    Write-Output 'Capture runtime archive verified.'
    return
}
[IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($target)) | Out-Null
$release = Invoke-RestMethod -Uri 'https://api.github.com/repos/z2476720314-sys/zshtrans/releases/tags/v3.4'
$asset = @($release.assets | Where-Object name -eq 'qq-runtime-9.9.30-48517.zip')
if ($asset.Count -ne 1) { throw 'Pinned capture runtime release asset was not found.' }
$staging = $target + '.tmp'
if (Test-Path -LiteralPath $staging) { throw 'Runtime staging path already exists.' }
try {
    Invoke-WebRequest -Uri $asset[0].url -Headers @{ Accept='application/octet-stream' } -OutFile $staging -UseBasicParsing
    if ((Get-FileHash -LiteralPath $staging -Algorithm SHA256).Hash -ne $expected) {
        throw 'Downloaded capture runtime SHA256 mismatch.'
    }
    Move-Item -LiteralPath $staging -Destination $target
} finally {
    if (Test-Path -LiteralPath $staging) { Remove-Item -LiteralPath $staging }
}
Write-Output 'Capture runtime archive downloaded and verified.'
