package phrases

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/local/zshtrans/internal/logx"
)

// Item 一条常用语（对应 pyc {id, desc, content}）
type Item struct {
	ID      string `json:"id"`
	Desc    string `json:"desc"`
	Content string `json:"content"`
}

var (
	mu    sync.RWMutex
	items []Item
	path  string
	seq   int
)

// Init 绑定文件路径并载入
func Init(p string) {
	mu.Lock()
	path = p
	mu.Unlock()
	Load()
}

// Load 读取常用语；坏文件回退空表（对应 pyc load_phrases）
func Load() []Item {
	mu.Lock()
	defer mu.Unlock()
	raw, err := os.ReadFile(path)
	if err != nil {
		items = []Item{}
		return items
	}
	var list []Item
	if err := json.Unmarshal(raw, &list); err != nil {
		logx.Warn("phrases read fail, start empty: %v", err)
		items = []Item{}
		return items
	}
	out := make([]Item, 0, len(list))
	for _, it := range list {
		if it.Content == "" && it.Desc == "" && it.ID == "" {
			continue
		}
		if it.ID == "" {
			it.ID = NewID()
		}
		out = append(out, it)
	}
	items = out
	return items
}

// List 当前常用语快照
func List() []Item {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Item, len(items))
	copy(out, items)
	return out
}

// Save 全量写回（对应 pyc save_phrases）
func Save(list []Item) error {
	mu.Lock()
	defer mu.Unlock()
	items = list
	raw, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Add 新增一条，返回新条目
func Add(desc, content string) (Item, error) {
	it := Item{ID: NewID(), Desc: desc, Content: content}
	list := append(List(), it)
	if err := Save(list); err != nil {
		return Item{}, err
	}
	return it, nil
}

// Update 按 id 修改
func Update(id, desc, content string) (bool, error) {
	list := List()
	found := false
	for i := range list {
		if list[i].ID == id {
			list[i].Desc = desc
			list[i].Content = content
			found = true
			break
		}
	}
	if !found {
		return false, nil
	}
	return true, Save(list)
}

// Delete 按 id 删除
func Delete(id string) (bool, error) {
	list := List()
	out := make([]Item, 0, len(list))
	found := false
	for _, it := range list {
		if it.ID == id {
			found = true
			continue
		}
		out = append(out, it)
	}
	if !found {
		return false, nil
	}
	return true, Save(out)
}

// NewID 生成稳定短 id（对应 pyc _new_phrase_id）
func NewID() string {
	mu.Lock()
	seq++
	n := seq
	mu.Unlock()
	return "p" + time.Now().Format("20060102150405") + "-" + itoa(n)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// Seed 首次运行写入内置种子常用语（对应 pyc _seed_bundled，已存在不覆盖）
func Seed() {
	if _, err := os.Stat(path); err == nil {
		return
	}
	seed := []Item{
		{ID: NewID(), Desc: "问候", Content: "Здравствуйте! Чем могу помочь?"},
		{ID: NewID(), Desc: "已发货", Content: "Ваш заказ отправлен. Пожалуйста, ожидайте доставку."},
		{ID: NewID(), Desc: "请稍等", Content: "Минуту, пожалуйста, уточняю информацию."},
		{ID: NewID(), Desc: "质量问题", Content: "Извините за доставленные неудобства. Мы обязательно решим этот вопрос."},
		{ID: NewID(), Desc: "感谢", Content: "Спасибо за ваш заказ! Будем рады видеть вас снова."},
	}
	if err := Save(seed); err != nil {
		logx.Warn("seed phrases fail: %v", err)
	}
}
