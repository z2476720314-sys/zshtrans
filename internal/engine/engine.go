package engine

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"sort"
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

// Rebuild 配置变更后隔离旧模型的缓存和正在执行的请求。
func (e *Engine) Rebuild(list []*providers.Provider, pref string, timeout int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.providers = list
	e.pref = pref
	e.timeout = timeout
	e.cache = newLRU(config.CacheMax)
	e.idx = 0
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
	return e.TranslateContext(context.Background(), text, src, tgt)
}

func (e *Engine) TranslateContext(ctx context.Context, text, src, tgt string) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	// 1. 空白折叠
	norm := strings.Join(strings.Fields(text), " ")
	e.mu.Lock()
	cache, pref, index := e.cache, strings.TrimSpace(e.pref), e.idx
	pool := make([]*providers.Provider, 0, len(e.providers))
	for _, p := range e.providers {
		if p.Enabled() {
			pool = append(pool, p)
		}
	}
	e.mu.Unlock()
	// 2. 缓存查询
	if hit, ok := cache.Get(cacheKey{norm, src, tgt}); ok {
		return Result{Text: hit, Provider: "cache", MS: 0}, nil
	}
	// 3. 提供方池
	if len(pool) == 0 {
		return Result{}, ErrNoProvider
	}

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
		start = index % len(ordered)
	}

	// 5. 轮转 + 健康优先
	order := append(append([]*providers.Provider{}, ordered[start:]...), ordered[:start]...)
	stableSortByHealth(order)

	var errs []string
	for _, p := range order {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		t0 := time.Now()
		out, err := safeCall(ctx, p.Fn, text, src, tgt)
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		if err != nil {
			p.MarkFail()
			errs = append(errs, fmt.Sprintf("%s: %v", p.Name, err))
			continue
		}
		p.MarkOK()
		e.mu.Lock()
		for i, item := range pool {
			if item == p && e.cache == cache {
				e.idx = (i + 1) % len(pool)
				break
			}
		}
		e.mu.Unlock()
		cache.Put(cacheKey{norm, src, tgt}, out)
		return Result{Text: out, Provider: p.Name, MS: int(time.Since(t0).Milliseconds())}, nil
	}
	return Result{}, fmt.Errorf("all providers failed:\n%s", strings.Join(errs, "\n"))
}

func safeCall(ctx context.Context, fn providers.Func, text, src, tgt string) (out string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return fn(ctx, text, src, tgt)
}

// stableSortByHealth 健康者优先，保持原有相对顺序。
//
// 必须按 key 稳定排序（对应 pyc `order.sort(key=lambda p: not p.healthy())`）。
// 原来的手写插入排序只在「前一个健康、当前不健康」时交换，等于只会把不健康的
// 往右推一格，无法把排在队首的不健康节点挪到后面——顺序不对，首选节点仍是坏的。
func stableSortByHealth(list []*providers.Provider) {
	sort.SliceStable(list, func(i, j int) bool {
		return list[i].Healthy() && !list[j].Healthy()
	})
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
