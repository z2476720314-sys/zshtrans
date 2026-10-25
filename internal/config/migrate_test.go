package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLegacyMigrationPreservesUserFiles(t *testing.T) {
	from, to := t.TempDir(), t.TempDir()
	config := []byte(`{"default_provider":"bailian:deepseek-v4.1-flash","api_keys":{"qwen":"test-value"},"providers":[{"id":"bailian","models":[{"id":"deepseek-v4.1-flash","output_limit":2048}]}]}`)
	phrases := []byte(`[{"id":"custom","desc":"售后","content":"请保留包装"}]`)
	for name, content := range map[string][]byte{"tt_config.json": config, "tt_phrases.json": phrases} {
		if err := os.WriteFile(filepath.Join(from, name), content, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := migrateLegacy(from, to); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string][]byte{"tt_config.json": config, "tt_phrases.json": phrases} {
		got, err := os.ReadFile(filepath.Join(to, name))
		if err != nil || string(got) != string(content) {
			t.Fatalf("migration changed %s", name)
		}
	}
	if err := os.WriteFile(filepath.Join(to, "tt_config.json"), []byte(`{"tgt_lang":"en"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := migrateLegacy(from, to); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(to, "tt_config.json"))
	if string(got) != `{"tgt_lang":"en"}` {
		t.Fatal("existing destination config overwritten")
	}
}

func TestInvalidLegacyMigrationDoesNotCreateDestination(t *testing.T) {
	from, to := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(from, "tt_config.json"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := migrateLegacy(from, to); err == nil {
		t.Fatal("invalid legacy file accepted")
	}
	if _, err := os.Stat(filepath.Join(to, "tt_config.json")); !os.IsNotExist(err) {
		t.Fatal("invalid legacy config copied")
	}
}
