param(
    [Parameter(Mandatory = $true)][string]$SourceDirectory,
    [string]$OutputPath = (Join-Path $PSScriptRoot '..\internal\capture\bundled\qq-runtime.zip')
)
$ErrorActionPreference = 'Stop'
$sourceRoot = (Resolve-Path -LiteralPath $SourceDirectory).Path
$manifestPath = Join-Path $sourceRoot 'native_manifest.json'
$manifest = Get-Content -LiteralPath $manifestPath -Raw -Encoding UTF8 | ConvertFrom-Json
$entries = @($manifest.files.PSObject.Properties | Sort-Object Name)
if ($manifest.qq_version -ne '9.9.30-48517' -or $entries.Count -ne 71) { throw 'Unexpected capture runtime manifest' }
foreach ($entry in $entries) {
    if ([IO.Path]::IsPathRooted($entry.Name) -or $entry.Name -match '(^|[\\/])\.\.([\\/]|$)' -or $entry.Name.Contains(':')) { throw 'Invalid manifest path' }
    $path = Join-Path $sourceRoot $entry.Name
    if ((Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant() -ne $entry.Value) { throw "Runtime hash mismatch: $($entry.Name)" }
}
$target = [IO.Path]::GetFullPath($OutputPath)
[IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($target)) | Out-Null
$staging = $target + '.tmp'
if (Test-Path -LiteralPath $staging) { throw "Staging file already exists: $staging" }
Add-Type -AssemblyName System.IO.Compression
Add-Type -AssemblyName System.IO.Compression.FileSystem
$archive = [IO.Compression.ZipFile]::Open($staging, [IO.Compression.ZipArchiveMode]::Create)
try {
    $paths = @($entries.Name) + @('native_manifest.json', 'QQ_CAPTURE_LICENSE')
    foreach ($relative in $paths) {
        $entry = $archive.CreateEntry($relative.Replace('\','/'), [IO.Compression.CompressionLevel]::Optimal)
        $entry.LastWriteTime = [DateTimeOffset]::new(1980,1,1,0,0,0,[TimeSpan]::Zero)
        $inputStream = [IO.File]::OpenRead((Join-Path $sourceRoot $relative))
        $outputStream = $entry.Open()
        try { $inputStream.CopyTo($outputStream) } finally { $inputStream.Dispose(); $outputStream.Dispose() }
    }
} finally { $archive.Dispose() }
Move-Item -LiteralPath $staging -Destination $target -Force
[pscustomobject]@{ Path=$target; Files=$paths.Count; Bytes=(Get-Item -LiteralPath $target).Length; SHA256=(Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash }
