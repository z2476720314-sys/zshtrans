package web

import (
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

func TestDesktopPageContainsItsAssetsWithoutServer(t *testing.T) {
	assets := fstest.MapFS{
		"index.html":     {Data: []byte(`<html><head><script type="module" crossorigin src="/assets/app.js"></script><link rel="stylesheet" crossorigin href="/assets/app.css"></head><body><div id="root"></div></body></html>`)},
		"assets/app.js":  {Data: []byte(`console.log("</script>")`)},
		"assets/app.css": {Data: []byte(`body{color:white}`)},
	}
	path, err := WriteDesktopPage(assets, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	page := string(raw)
	if strings.Contains(page, `src="/assets/`) || strings.Contains(page, `href="/assets/`) || !strings.Contains(page, `body{color:white}`) || !strings.Contains(page, `connect-src 'none'`) {
		t.Fatal("desktop page still requires an asset server or allows network access")
	}
	if strings.Count(page, "</script>") != 1 {
		t.Fatal("embedded script can terminate its HTML tag")
	}
}
