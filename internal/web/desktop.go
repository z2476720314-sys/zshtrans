package web

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var scriptAsset = regexp.MustCompile(`<script[^>]* src="([^"]+)"[^>]*></script>`)
var styleAsset = regexp.MustCompile(`<link[^>]* href="([^"]+)"[^>]*>`)
var scriptEnd = regexp.MustCompile(`(?i)</script`)

// WriteDesktopPage keeps the bundled UI local and self-contained, without HTTP.
func WriteDesktopPage(assets fs.FS, directory string) (string, error) {
	raw, err := fs.ReadFile(assets, "index.html")
	if err != nil {
		return "", err
	}
	page := string(raw)
	var assetErr error
	inline := func(pattern *regexp.Regexp, tag string) {
		page = pattern.ReplaceAllStringFunc(page, func(element string) string {
			path := strings.TrimPrefix(pattern.FindStringSubmatch(element)[1], "/")
			content, err := fs.ReadFile(assets, path)
			if err != nil {
				assetErr = err
				return ""
			}
			if tag == "script" {
				return `<script type="module">` + scriptEnd.ReplaceAllString(string(content), `<\/script`) + `</script>`
			}
			return `<style>` + string(content) + `</style>`
		})
	}
	inline(scriptAsset, "script")
	inline(styleAsset, "style")
	if assetErr != nil {
		return "", assetErr
	}
	page = strings.Replace(page, "<head>", `<head><meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data: blob:; font-src data:; connect-src 'none'; base-uri 'none'; form-action 'none'">`, 1)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(directory, "index.html")
	if err := os.WriteFile(path, []byte(page), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// The raw WebView bindings exist on each page; only this local top-level page gets the session capability.
func DesktopScript(pageURL, token string) string {
	page, _ := json.Marshal(pageURL)
	key, _ := json.Marshal(token)
	return fmt.Sprintf(`(() => {
  if (window !== window.top || location.href.split('#')[0] !== %s) return;
  const token = %s;
  window.zshDesktop = {
    request: (id, method, path, body) => window._zshRequest(token, id, method, path, body),
    cancel: (id) => window._zshCancel(token, id)
  };
})();`, page, key)
}
