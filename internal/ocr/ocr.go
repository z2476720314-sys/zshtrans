package ocr

import (
	"errors"
	"fmt"
	"image"
	"sort"
	"strings"
	"sync"

	"github.com/local/zshtrans/internal/logx"
)

// Result 一次 OCR 的结果
type Result struct {
	Text   string `json:"text"`
	Engine string `json:"engine"`
	MS     int    `json:"ms"`
}

// Keys 密钥来源
type Keys struct {
	Space     string
	BaiduAK   string
	BaiduSK   string
	SrcLang   string
	Vision    VisionFn
	VisionTag string
}

// cache OCR 结果缓存（对应 pyc _OCR_CACHE）
type cache struct {
	mu    sync.Mutex
	items map[string]string
	cap   int
	order []string
}

var ocrCache = &cache{items: map[string]string{}, cap: 60}

func (c *cache) get(k string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.items[k]
	return v, ok
}

func (c *cache) put(k, v string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.items[k]; !ok {
		c.order = append(c.order, k)
	}
	c.items[k] = v
	for len(c.order) > c.cap {
		old := c.order[0]
		c.order = c.order[1:]
		delete(c.items, old)
	}
}

// Run 多引擎并行 + 中位数择优（对应 pyc run_ocr）
func Run(img image.Image, keys Keys) (Result, error) {
	cands := Candidates(img)
	if len(cands) == 0 {
		return Result{}, errors.New("no image candidate")
	}

	type outcome struct {
		name string
		cand string
		text string
		err  error
	}

	var collected []outcome
	var hits []outcome
	var errs []string
	var mu sync.Mutex
	var wg sync.WaitGroup

	// 每个候选图跑所有可用引擎
	for _, cand := range cands {
		cand := cand
		b64 := B64(cand.PNG)
		if hit, ok := ocrCache.get("raw:" + hashKey(b64)); ok {
			hits = append(hits, outcome{name: "cache", text: hit})
			continue
		}
		type engine struct {
			name string
			fn   func(string) (string, error)
		}
		engines := make([]engine, 0, 3)
		if keys.Vision != nil {
			tag := keys.VisionTag
			if tag == "" {
				tag = "vision"
			}
			engines = append(engines, engine{tag, func(s string) (string, error) { return Vision(s, keys.Vision) }})
		}
		if strings.TrimSpace(keys.Space) != "" {
			engines = append(engines, engine{"ocr.space", func(s string) (string, error) { return Space(s, keys.Space, keys.SrcLang) }})
		}
		if strings.TrimSpace(keys.BaiduAK) != "" && strings.TrimSpace(keys.BaiduSK) != "" {
			engines = append(engines, engine{"baidu-ocr", func(s string) (string, error) { return Baidu(s, keys.BaiduAK, keys.BaiduSK) }})
		}
		if len(engines) == 0 {
			return Result{}, errors.New("no OCR engine available (set a vision model or OCR key in settings)")
		}
		for _, eng := range engines {
			eng := eng
			wg.Add(1)
			go func() {
				defer wg.Done()
				text, err := eng.fn(b64)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					logx.Warn("ocr %s [%s]: %v", eng.name, cand.Name, err)
					errs = append(errs, fmt.Sprintf("%s [%s]: %v", eng.name, cand.Name, err))
					return
				}
				if strings.TrimSpace(text) == "" {
					logx.Warn("ocr %s [%s]: no text -> next candidate", eng.name, cand.Name)
					errs = append(errs, fmt.Sprintf("%s [%s]: no text recognized", eng.name, cand.Name))
					return
				}
				collected = append(collected, outcome{name: eng.name, cand: cand.Name, text: text})
			}()
		}
	}
	wg.Wait()
	if len(hits) > 0 {
		best := hits[0]
		return Result{Text: best.text, Engine: best.name}, nil
	}

	if len(collected) == 0 {
		// 对齐 pyc：失败信息带图像尺寸/候选数/背景/缩放，并逐引擎列出原因。
		// bg 现在是真实的 Otsu 阈值（原版写死 255，排查时看不出图像明暗）。
		detail := strings.Join(errs, "\n")
		if detail == "" {
			detail = "no engine produced text"
		}
		bg := Otsu(Gray(img))
		// x 是候选里最大的放大倍数（原版写死 1.0，多候选时看不出放大了几倍）
		scale := 1.0
		for _, c := range cands {
			if c.Scale > scale {
				scale = c.Scale
			}
		}
		return Result{}, fmt.Errorf("all OCR failed (img %dx%d, %d cands, bg=%d, x%.1f):\n%s",
			img.Bounds().Dx(), img.Bounds().Dy(), len(cands), bg, scale, detail)
	}

	// 中位数长度择优（对应 pyc _ocr_median）
	sort.SliceStable(collected, func(i, j int) bool {
		return len([]rune(collected[i].text)) < len([]rune(collected[j].text))
	})
	best := collected[len(collected)/2]
	// 缓存键必须跟着「产出 best 的那个候选图」走。原来恒用 cands[0]，
	// 若 best 来自 binary/scaled 候选，下次会拿原图的键命中错误结果、并跳过更优候选。
	keyCand := cands[0]
	for _, c := range cands {
		if c.Name == best.cand {
			keyCand = c
			break
		}
	}
	ocrCache.put("raw:"+hashKey(B64(keyCand.PNG)), best.text)
	return Result{Text: best.text, Engine: best.name}, nil
}

// hashKey 轻量内容指纹，避免依赖 crypto 开销
func hashKey(s string) string {
	var h uint64 = 1469598103934665603
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	var buf [16]byte
	for i := 0; i < 16; i++ {
		buf[i] = byte(h >> (uint(i) * 4))
	}
	return string(buf[:])
}
