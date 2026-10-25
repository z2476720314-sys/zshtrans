package ocr

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"image"
	"sort"
	"strings"
	"sync"
	"time"

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
	Space         string
	BaiduAK       string
	BaiduSK       string
	SrcLang       string
	Vision        VisionFn
	VisionContext func(context.Context, string) (string, error)
	VisionTag     string
	VisionScope   string
}

// cache OCR 结果缓存（对应 pyc _OCR_CACHE）
type cache struct {
	mu         sync.Mutex
	items      map[string]string
	cap        int
	order      []string
	generation uint64
}

var ocrCache = &cache{items: map[string]string{}, cap: 60}

func (c *cache) get(k string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.items[k]
	return v, ok
}

func (c *cache) put(k, v string, generation uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.generation != generation {
		return
	}
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

// ResetCache 隔离设置变更前的识别结果，包括仍在执行的请求。
func ResetCache() {
	ocrCache.mu.Lock()
	defer ocrCache.mu.Unlock()
	ocrCache.generation++
	ocrCache.items = map[string]string{}
	ocrCache.order = nil
}

// Run 同一候选内多引擎并行；未识别到文字才尝试下一候选。
func Run(img image.Image, keys Keys) (Result, error) {
	return RunContext(context.Background(), img, keys)
}

func RunContext(ctx context.Context, img image.Image, keys Keys) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	started := time.Now()
	if img == nil {
		return Result{}, errors.New("no image candidate")
	}
	type engine struct {
		name string
		fn   func(string) (string, error)
	}
	engines := make([]engine, 0, 3)
	if keys.Vision != nil || keys.VisionContext != nil {
		tag := keys.VisionTag
		if tag == "" {
			tag = "vision"
		}
		engines = append(engines, engine{tag, func(s string) (string, error) {
			if keys.VisionContext != nil {
				return keys.VisionContext(ctx, s)
			}
			return Vision(s, keys.Vision)
		}})
	}
	if strings.TrimSpace(keys.Space) != "" {
		engines = append(engines, engine{"ocr.space", func(s string) (string, error) { return SpaceContext(ctx, s, keys.Space, keys.SrcLang) }})
	}
	if strings.TrimSpace(keys.BaiduAK) != "" && strings.TrimSpace(keys.BaiduSK) != "" {
		engines = append(engines, engine{"baidu-ocr", func(s string) (string, error) { return BaiduContext(ctx, s, keys.BaiduAK, keys.BaiduSK) }})
	}
	if len(engines) == 0 {
		return Result{}, errors.New("no OCR engine available (set a vision model or OCR key in settings)")
	}
	raw, err := EncodePNG(img)
	if err != nil {
		return Result{}, err
	}
	ocrCache.mu.Lock()
	generation := ocrCache.generation
	ocrCache.mu.Unlock()
	scope := fmt.Sprintf("%d\x00%s\x00%s\x00%s\x00%t\x00%s\x00%s\x00%s", generation, keys.SrcLang, keys.VisionTag, keys.VisionScope, keys.Vision != nil || keys.VisionContext != nil, keys.Space, keys.BaiduAK, keys.BaiduSK)
	key := hashKey(scope + "\x00" + string(raw))
	if hit, ok := ocrCache.get(key); ok {
		return Result{Text: hit, Engine: "cache"}, nil
	}
	type outcome struct {
		name, text string
		err        error
	}
	var errs []string
	cands := Candidates(img)
	for _, cand := range cands {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		b64 := B64(cand.PNG)
		results := make([]outcome, len(engines))
		var wg sync.WaitGroup
		for i, eng := range engines {
			wg.Add(1)
			go func() {
				defer wg.Done()
				text, err := eng.fn(b64)
				results[i] = outcome{eng.name, strings.TrimSpace(text), err}
			}()
		}
		wg.Wait()
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		var collected []outcome
		for _, result := range results {
			if result.err == nil && result.text != "" {
				collected = append(collected, result)
				continue
			}
			err := result.err
			if err == nil {
				err = errors.New("no text recognized")
			}
			logx.Warn("ocr %s [%s]: %v", result.name, cand.Name, err)
			errs = append(errs, fmt.Sprintf("%s [%s]: %v", result.name, cand.Name, err))
		}
		if len(collected) == 0 {
			continue
		}
		sort.SliceStable(collected, func(i, j int) bool { return len([]rune(collected[i].text)) < len([]rune(collected[j].text)) })
		best := collected[len(collected)/2]
		ocrCache.put(key, best.text, generation)
		return Result{Text: best.text, Engine: best.name, MS: int(time.Since(started).Milliseconds())}, nil
	}
	scale := 1.0
	for _, cand := range cands {
		scale = max(scale, cand.Scale)
	}
	return Result{}, fmt.Errorf("all OCR failed (img %dx%d, %d cands, bg=%d, x%.1f):\n%s", img.Bounds().Dx(), img.Bounds().Dy(), len(cands), Otsu(Gray(img)), scale, strings.Join(errs, "\n"))
}

// hashKey 缓存中只保存内容与配置指纹，不保留凭据。
func hashKey(s string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(s)))
}
