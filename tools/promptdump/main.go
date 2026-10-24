// promptdump 把 BuildPrompt 的全部语向组合导出为 JSON，供提示词调参脚本使用。
// 仅在调参期间存在，不参与发布构建。
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/local/zshtrans/internal/providers"
)

func main() {
	srcs := []string{"auto", "ru", "zh", "en", "ja"}
	tgts := []string{"ru", "zh", "en", "ja"}
	out := map[string]string{}
	for _, s := range srcs {
		for _, t := range tgts {
			if s == t {
				continue
			}
			out[s+"->"+t] = providers.BuildPrompt(s, t)
		}
	}
	raw, err := json.MarshalIndent(out, "", " ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(string(raw))
}
