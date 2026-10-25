//go:build windows

package update

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestInstallRollbackPreservesSpecialPath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "中文 %TEMP% O'Brien & files")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	self := filepath.Join(dir, "app.exe")
	newExe := filepath.Join(dir, "download.exe")
	old := []byte("original executable")
	if err := os.WriteFile(self, old, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newExe, []byte("not executable"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-EncodedCommand", encodedCommand(installScript(self, newExe, 0, nil)))
	hideInstaller(cmd)
	if err := cmd.Run(); err == nil {
		t.Fatal("invalid executable installation should fail")
	}
	got, err := os.ReadFile(self)
	if err != nil || string(got) != string(old) {
		t.Fatalf("rollback lost original: %q, %v", got, err)
	}
	if _, err := os.Stat(self + ".update-error.txt"); err != nil {
		t.Fatal("failure diagnostic missing")
	}
}

func TestInstallReplacesAndKeepsRollbackCopy(t *testing.T) {
	dir := t.TempDir()
	self := filepath.Join(dir, "installed.exe")
	source, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(self, []byte("previous version"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZSHTRANS_UPDATE_TEST_CHILD", "1")
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-EncodedCommand", encodedCommand(installScript(self, source, 0, []string{"-test.run=^TestInstallChildProcess$"})))
	hideInstaller(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		diagnostic, _ := os.ReadFile(self + ".update-error.txt")
		t.Fatalf("installer: %v %s %s", err, out, diagnostic)
	}
	old, err := os.ReadFile(self + ".old")
	if err != nil || string(old) != "previous version" {
		t.Fatalf("rollback copy missing: %v", err)
	}
	st, err := os.Stat(self)
	if err != nil || st.Size() < 1024 {
		t.Fatalf("new image missing: %v", err)
	}
}

func TestInstallChildProcess(t *testing.T) {
	if os.Getenv("ZSHTRANS_UPDATE_TEST_CHILD") == "1" {
		os.Exit(0)
	}
}
