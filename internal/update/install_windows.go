//go:build windows

package update

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf16"
)

func Apply(newExe string) error {
	if err := checkPE(newExe); err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	self, err = filepath.Abs(self)
	if err != nil {
		return err
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-EncodedCommand", encodedCommand(installScript(self, newExe, os.Getpid(), os.Args[1:])))
	hideInstaller(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动更新程序失败：%w", err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func hideInstaller(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}

func encodedCommand(script string) string {
	units := utf16.Encode([]rune(script))
	buf := make([]byte, len(units)*2)
	for i, unit := range units {
		binary.LittleEndian.PutUint16(buf[i*2:], unit)
	}
	return base64.StdEncoding.EncodeToString(buf)
}

func installScript(self, source string, waitPID int, args []string) string {
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
	quotedArgs := make([]string, len(args))
	for i, arg := range args {
		quotedArgs[i] = syscall.EscapeArg(arg)
	}
	return fmt.Sprintf(`$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
function FileHash([string] $path) {
  $stream = [IO.File]::OpenRead($path)
  $hash = [Security.Cryptography.SHA256]::Create()
  try { return [BitConverter]::ToString($hash.ComputeHash($stream)) } finally { $stream.Dispose(); $hash.Dispose() }
}
$target = %s
$source = %s
$arguments = %s
$backup = $target + '.old'
$staged = $target + '.new'
$moved = $false
try {
  if (%d -gt 0) {
    $running = Get-Process -Id %d -ErrorAction SilentlyContinue
    if ($running -and -not $running.WaitForExit(30000)) { throw 'Application did not exit; original file was preserved.' }
  }
  Copy-Item -LiteralPath $source -Destination $staged -Force
  if ((FileHash $source) -ne (FileHash $staged)) { throw 'Staged update checksum mismatch.' }
  if (Test-Path -LiteralPath $backup) { Remove-Item -LiteralPath $backup -Force }
  Move-Item -LiteralPath $target -Destination $backup
  $moved = $true
  Move-Item -LiteralPath $staged -Destination $target
  $start = New-Object System.Diagnostics.ProcessStartInfo
  $start.FileName = $target
  $start.Arguments = $arguments
  $start.WorkingDirectory = [IO.Path]::GetDirectoryName($target)
  $start.UseShellExecute = $false
  $start.CreateNoWindow = $true
  $child = [Diagnostics.Process]::Start($start)
  if ($child.WaitForExit(1000) -and $child.ExitCode -ne 0) { throw 'Updated application failed to start.' }
  if (Test-Path -LiteralPath ($target + '.update-error.txt')) { Remove-Item -LiteralPath ($target + '.update-error.txt') -Force }
  exit 0
} catch {
  $failure = $_.Exception.Message
  if ($moved -and (Test-Path -LiteralPath $backup)) {
    if (Test-Path -LiteralPath $target) { Remove-Item -LiteralPath $target -Force -ErrorAction SilentlyContinue }
    Move-Item -LiteralPath $backup -Destination $target -Force
  }
  [IO.File]::WriteAllText($target + '.update-error.txt', $failure, [Text.Encoding]::UTF8)
  exit 1
} finally {
  if (Test-Path -LiteralPath $staged) { Remove-Item -LiteralPath $staged -Force -ErrorAction SilentlyContinue }
}
`, quote(self), quote(source), quote(strings.Join(quotedArgs, " ")), waitPID, waitPID)
}
