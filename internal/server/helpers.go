package server

import (
	"encoding/base64"
	"os"

	"github.com/local/zshtrans/internal/config"
	"github.com/local/zshtrans/internal/hotkeys"
)

func decodeB64(s string) ([]byte, error) {
	if raw, err := base64.StdEncoding.DecodeString(s); err == nil {
		return raw, nil
	}
	return base64.RawStdEncoding.DecodeString(s)
}

func envOrNil() func(string) string { return os.Getenv }

// validateHotkeys 三项热键格式与冲突校验
func validateHotkeys(h config.Hotkeys) string {
	if err := hotkeys.Validate(h.Selection, h.Screenshot, h.Window); err != nil {
		return err.Error()
	}
	return ""
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
