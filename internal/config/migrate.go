package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// MigrateLegacy imports the previous desktop profile only into a new portable profile.
func MigrateLegacy() error {
	if os.Getenv("ZSHSTRANS_CONFIG") != "" {
		return nil
	}
	return migrateLegacy(userConfigDir(), BaseDir())
}

func migrateLegacy(from, to string) error {
	if from == "" || from == to {
		return nil
	}
	if _, err := os.Stat(filepath.Join(to, "tt_config.json")); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	configRaw, err := os.ReadFile(filepath.Join(from, "tt_config.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !json.Valid(configRaw) {
		return errors.New("旧版配置格式无效，未执行迁移")
	}
	phrasesPath := filepath.Join(to, "tt_phrases.json")
	if _, err := os.Stat(phrasesPath); errors.Is(err, os.ErrNotExist) {
		raw, readErr := os.ReadFile(filepath.Join(from, "tt_phrases.json"))
		if readErr == nil {
			if !json.Valid(raw) {
				return errors.New("旧版常用语格式无效，未执行迁移")
			}
			if err := writeAtomic(phrasesPath, raw); err != nil {
				return err
			}
		} else if !errors.Is(readErr, os.ErrNotExist) {
			return readErr
		}
	} else if err != nil {
		return err
	}
	if err := writeAtomic(filepath.Join(to, "tt_config.json"), configRaw); err != nil {
		return fmt.Errorf("迁移旧版配置失败: %w", err)
	}
	return nil
}
