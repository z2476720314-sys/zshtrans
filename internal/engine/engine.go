package engine

import (
	"container/list"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/local/zshtrans/internal/config"
	"github.com/local/zshtrans/internal/providers"
)

// ErrNoProvider 对应 pyc 'no provider enabled'
var ErrNoProvider = errors.New("no provider enabled")

// Result 翻译结果（对应 pyc {text, provider, ms}）
type Result struct {
	Text     string `json:"text"`
	Provider string `json:"provider"`
	MS       int    `json:"ms"`
}

// Engine 翻译引擎（对应 pyc TranslateEngine）
type Engine struct {
	mu        sync.Mutex
	providers []*providers.Provider
	idx       int
	cache     *lru
	pref      string
	timeout   int
}

// New 构建引擎
func New(list []*providers.Provider, pref string, timeout int) *Engine {
	return &Engine{
		providers: list,
		cache:     newLRU(config.CacheMax),
		pref:      pref,
		timeout:   timeout,
	}
}

// Rebuild 配置变更后重建（保留健康度与缓存）
func (e *Engine) Rebuild(list []*providers.Provider, pref string, timeout int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.providers = list
	e.pref = pref
	e.timeout = timeout
	if e.cache == nil {
		e.cache = newLRU(config.CacheMax)
	}
}

// Pool 已启用的提供方（对应 pyc pool()）
func (e *Engine) Pool() []*providers.Provider {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]*providers.Provider, 0, len(e.providers))
	for _, p := range e.providers {
		if p.Enabled() {
			out = append(out, p)
		}
	}
	return out
}

// Status 健康度快照（GET /api/status）
type Status struct {
	Name    string `json:"name"`
	Label   string `json:"label"`
	Healthy bool   `json:"healthy"`
}

// Statuses 全量状态
func (e *Engine) Statuses() []Status {
	pool := e.Pool()
	out := make([]Status, 0, len(pool))
	for _, p := range pool {
		out = append(out, Status{Name: p.Name, Label: providers.Label(p.Name), Healthy: p.Healthy()})
	}
	return out
}

// Translate 主翻译流程，严格复刻 pyc TranslateEngine.translate
func (e *Engine) Translate(text, src, tgt string) (Result, error) {
	// 1. 空白折叠
	norm := strings.Join(strings.Fields(text), " ")
	// 2. 缓存查询
	if hit, ok := e.cache.Get(cacheKey{norm, src, tgt}); ok {
		return Result{Text: hit, Provider: "cache", MS: 0}, nil
	}
	// 3. 提供方池
	pool := e.Pool()
	if len(pool) == 0 {
		return Result{}, ErrNoProvider
	}

	e.mu.Lock()
	pref := strings.TrimSpace(e.pref)
	var prefPool, rest []*providers.Provider
	if pref != "" {
		for _, p := range pool {
			if p.Name == pref {
				prefPool = append(prefPool, p)
			} else {
				rest = append(rest, p)
			}
		}
	}
	ordered := pool
	if len(prefPool) > 0 {
		ordered = append(append([]*providers.Provider{}, prefPool...), rest...)
	}
	var start int
	if len(prefPool) == 0 {
		start = e.idx % len(ordered)
	}
	e.mu.Unlock()

	// 5. 轮转 + 健康优先
	order := append(append([]*providers.Provider{}, ordered[start:]...), ordered[:start]...)
	stableSortByHealth(order)

	var errs []string
	for _, p := range order {
		t0 := time.Now()
		out, err := safeCall(p.Fn, text, src, tgt)
		if err != nil {
			p.MarkFail()
			errs = append(errs, fmt.Sprintf("%s: %v", p.Name, err))
			continue
		}
		p.MarkOK()
		e.mu.Lock()
		for i, item := range pool {
			if item == p {
				e.idx = (i + 1) % len(pool)
				break
			}
		}
		e.mu.Unlock()
		e.cache.Put(cacheKey{norm, src, tgt}, out)
		return Result{Text: out, Provider: p.Name, MS: int(time.Since(t0).Milliseconds())}, nil
	}
	return Result{}, fmt.Errorf("all providers failed:\n%s", strings.Join(errs, "\n"))
}

func safeCall(fn providers.Func, text, src, tgt string) (out string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return fn(text, src, tgt)
}

// stableSortByHealth 健康者优先，保持原有相对顺序
func stableSortByHealth(list []*providers.Provider) {
	for i := 1; i < len(list); i++ {
		for j := i; j > 0 && !list[j].Healthy() && list[j-1].Healthy(); j-- {
			list[j], list[j-1] = list[j-1], list[j]
		}
	}
}

// ---------- LRU 缓存（对应 pyc _cache_get/_cache_put，容量 300） ----------

type cacheKey struct {
	text string
	src  string
	tgt  string
}

type lruEntry struct {
	key   cacheKey
	value string
}

type lru struct {
	mu    sync.Mutex
	cap   int
	items map[cacheKey]*list.Element
	order *list.List
}

func newLRU(capacity int) *lru {
	if capacity <= 0 {
		capacity = 300
	}
	return &lru{cap: capacity, items: make(map[cacheKey]*list.Element), order: list.New()}
}

func (c *lru) Get(k cacheKey) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[k]; ok {
		c.order.MoveToFront(el)
		return el.Value.(*lruEntry).value, true
	}
	return "", false
}

func (c *lru) Put(k cacheKey, v string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[k]; ok {
		el.Value.(*lruEntry).value = v
		c.order.MoveToFront(el)
		return
	}
	el := c.order.PushFront(&lruEntry{key: k, value: v})
	c.items[k] = el
	for c.order.Len() > c.cap {
		back := c.order.Back()
		if back == nil {
			break
		}
		c.order.Remove(back)
		delete(c.items, back.Value.(*lruEntry).key)
	}
}

// Size 缓存条数（线程安全）
func (c *lru) Size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

// Len 缓存条数（自检用）
func (e *Engine) Len() int {
	e.mu.Lock()
	c := e.cache
	e.mu.Unlock()
	if c == nil {
		return 0
	}
	return c.Size()
}
