// Package update 自更新：检查 GitHub Releases → 下载 → 替换自身 exe → 重启。
//
// 关键约束：Windows 上运行中的 exe 无法被覆盖或删除。所以更新流程是
//  1. 新版本下载到临时目录；
//  2. 启动无窗口 PowerShell，等待本进程正常退出；
//  3. 同目录暂存、校验、替换并重启，保留 .old 供回滚。
//
// 用户若选择「稍后」，只记录版本号，下次启动再问；「跳过此版本」则写进配置不再提示。
package update

import (
	"context"
	"crypto/sha256"
	"debug/pe"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/local/zshtrans/internal/config"
)

// Repo 默认 GitHub 仓库（owner/name）
const Repo = "z2476720314-sys/zshtrans"

// AssetName 发布包文件名（GitHub Release 里上传的 exe 名）
const AssetName = "zshtrans.exe"

// Release GitHub Release 摘要
type Release struct {
	Tag         string  `json:"tag_name"`
	Name        string  `json:"name"`
	Body        string  `json:"body"`
	HTMLURL     string  `json:"html_url"`
	PublishedAt string  `json:"published_at"`
	Draft       bool    `json:"draft"`
	Prerelease  bool    `json:"prerelease"`
	Assets      []Asset `json:"assets"`
	// 计算字段
	Version string `json:"version"` // 去掉前缀 v 的版本号
	URL     string `json:"url"`     // 首选下载地址
	// URLs 备选下载地址（按顺序尝试）。第一个是 api.github.com 的资源端点，
	// 第二个是 github.com 的 browser_download_url。
	URLs   []string `json:"urls"`
	Size   int64    `json:"size"`
	Digest string   `json:"digest"`
}

// Asset Release 附件
type Asset struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
	// URL 是 api.github.com 的资源端点，需带 Accept: application/octet-stream
	// 才会 302 到真实文件。国内网络到 github.com 常连不上，但到 api.github.com
	// 与 release-assets.githubusercontent.com 是通的，所以这个端点才是主路径。
	URL         string `json:"url"`
	DownloadURL string `json:"browser_download_url"`
}

// ErrNoRelease 仓库还没有任何 release
var ErrNoRelease = errors.New("仓库暂无发布版本")

// client 独立超时（更新检查不该被翻译的 8s 超时绑住）
var client = &http.Client{Timeout: 20 * time.Second}

// downloadClient 下载专用：整个响应体的传输可能几十秒，不能沿用 20s 总超时
// （实测 10MB 包在国内网络 20s 只到 1.2MB 就 Client.Timeout 中断）。
// 总超时覆盖响应体停滞，主动取消通过 request context 传递。
var downloadClient = &http.Client{
	Timeout: 3 * time.Minute,
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 20 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:   20 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConnsPerHost:   4,
	},
}

// Check 拉取 latest release 并与当前版本比较。返回 nil 表示已是最新。
func Check(current string) (*Release, error) { return CheckRepo(Repo, current) }

// CheckRepo 指定仓库检查更新。
//
// 不用 /releases/latest：GitHub 在该接口上按 created_at 取「最新」，同一天连发两个
// release 时排序不稳定，可能返回较旧的那个（实测 v3.2 建好后 latest 仍指向 v3.1）。
// 改为拉 release 列表，自己按版本号取最大，并跳过 draft / prerelease。
func CheckRepo(repo, current string) (*Release, error) {
	if strings.TrimSpace(repo) == "" {
		repo = Repo
	}
	url := "https://api.github.com/repos/" + repo + "/releases?per_page=30"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "zshtrans-updater")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("检查更新失败：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNoRelease
	}
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("检查更新失败：HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var list []Release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&list); err != nil {
		return nil, fmt.Errorf("解析发布信息失败：%w", err)
	}
	if len(list) == 0 {
		return nil, ErrNoRelease
	}

	// 挑出正式发布里版本号最大的那个
	best := pickLatest(list)
	if best == nil {
		return nil, ErrNoRelease
	}

	for _, a := range best.Assets {
		if !strings.EqualFold(a.Name, AssetName) {
			continue
		}
		best.Size = a.Size
		best.Digest = a.Digest
		// 顺序有讲究：api.github.com 的资源端点在国内网络可达，
		// github.com 的 browser_download_url 经常直接连不上（实测超时 8s 无响应）。
		if a.URL != "" {
			best.URLs = append(best.URLs, a.URL)
		}
		if a.DownloadURL != "" {
			best.URLs = append(best.URLs, a.DownloadURL)
		}
		break
	}
	if len(best.URLs) == 0 {
		return nil, fmt.Errorf("发布 %s 未附带 %s", best.Tag, AssetName)
	}
	best.URL = best.URLs[0]
	if CompareVersions(best.Version, TrimVersion(current)) <= 0 {
		return nil, nil
	}
	return best, nil
}

// pickLatest 从发布列表里挑版本号最大的正式版（跳过 draft / prerelease）。
// 抽成独立函数是为了能单测：GitHub 的 /releases/latest 排序不可靠，这段逻辑是
// 「检查更新」正确性的核心，必须被测试锁住。
func pickLatest(list []Release) *Release {
	var best *Release
	for i := range list {
		rel := list[i]
		if rel.Draft || rel.Prerelease {
			continue
		}
		rel.Version = TrimVersion(rel.Tag)
		if best == nil || CompareVersions(rel.Version, best.Version) > 0 {
			cp := rel
			best = &cp
		}
	}
	return best
}

// TrimVersion 去掉 v/V 前缀与首尾空白
func TrimVersion(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	return v
}

var verRe = regexp.MustCompile(`\d+`)

// CompareVersions 语义化比较（只比数字段，缺失按 0）。
// 返回 1 表示 a 更新，-1 表示 a 更旧，0 相同。
func CompareVersions(a, b string) int {
	pa, pb := verRe.FindAllString(a, -1), verRe.FindAllString(b, -1)
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		x, y := 0, 0
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x > y {
			return 1
		}
		if x < y {
			return -1
		}
	}
	return 0
}

// Download 下载新版本到 dest（带进度回调，可为 nil）。
//
// 到 github.com / release-assets.githubusercontent.com 的连接在国内网络经常
// 握手超时或传输中途 wsarecv 断流（实测同一 URL 三次里两次失败），而更新是
// 一次性、不可交互的操作，失败后用户只能手动点重试。所以这里做有限次重试，
// 每次从 0 重新下载（不支持 Range 续传的镜像很多，续传反而更容易写坏文件）。
func Download(url, dest string, progress func(done, total int64)) error {
	return DownloadAny([]string{url}, dest, progress)
}

// DownloadAny 依次尝试多个下载地址，任一成功即返回。
//
// 传入 rel.URLs 时首选 api.github.com 的资源端点（国内可达），失败再退回
// github.com 的 browser_download_url。
func DownloadAny(urls []string, dest string, progress func(done, total int64)) error {
	return downloadAny(context.Background(), urls, dest, progress)
}

// DownloadRelease 仅将与 GitHub 附件大小及 SHA256 一致的文件标记为可安装。
func DownloadRelease(ctx context.Context, rel *Release, dest string, progress func(done, total int64)) error {
	if rel == nil || rel.Size <= 0 || !regexp.MustCompile(`^sha256:[a-fA-F0-9]{64}$`).MatchString(rel.Digest) {
		return errors.New("发布附件缺少有效的大小或 SHA256，不能安装")
	}
	if err := downloadAny(ctx, rel.URLs, dest, progress); err != nil {
		return err
	}
	if err := VerifyRelease(rel, dest); err != nil {
		_ = os.Remove(dest)
		return err
	}
	return nil
}

// VerifyRelease 在安装前再次校验，防止已下载文件被改动。
func VerifyRelease(rel *Release, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return err
	}
	if rel == nil || n != rel.Size {
		return errors.New("安装包大小与发布信息不一致，请重新下载")
	}
	if !strings.EqualFold("sha256:"+fmt.Sprintf("%x", h.Sum(nil)), rel.Digest) {
		return errors.New("安装包 SHA256 不匹配，请重新下载")
	}
	return checkPE(path)
}

func downloadAny(ctx context.Context, urls []string, dest string, progress func(done, total int64)) error {
	var list []string
	for _, u := range urls {
		if strings.TrimSpace(u) != "" {
			list = append(list, u)
		}
	}
	if len(list) == 0 {
		return errors.New("下载地址为空")
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	var errs []string
	for _, u := range list {
		err := downloadRetry(ctx, u, dest, progress)
		if err == nil {
			return nil
		}
		errs = append(errs, hostOf(u)+": "+err.Error())
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	if len(errs) == 1 {
		return errors.New(errs[0])
	}
	return fmt.Errorf("所有下载地址均失败：\n%s", strings.Join(errs, "\n"))
}

// hostOf 取 URL 的主机名，用于错误信息里区分是哪个源失败
func hostOf(raw string) string {
	if i := strings.Index(raw, "://"); i >= 0 {
		raw = raw[i+3:]
	}
	if i := strings.IndexAny(raw, "/?#"); i >= 0 {
		raw = raw[:i]
	}
	return raw
}

// downloadRetry 对单个地址做有限次重试
func downloadRetry(ctx context.Context, url, dest string, progress func(done, total int64)) error {
	const attempts = 3
	var lastErr error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			// 指数退避，给不稳定的出口一点恢复时间
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(i) * 2 * time.Second):
			}
			if progress != nil {
				progress(0, 0) // 重置进度显示
			}
		}
		err := downloadOnce(ctx, url, dest, progress)
		if err == nil {
			return nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return fmt.Errorf("下载失败（已重试 %d 次）：%w", attempts, lastErr)
}

// downloadOnce 单次下载：写 .part → 校验长度/非空/PE 头 → 改名
func downloadOnce(ctx context.Context, url, dest string, progress func(done, total int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "zshtrans-updater")
	// api.github.com 的资源端点只在带这个 Accept 时才 302 到真实文件；
	// 不带会返回一小段 JSON 元数据，被 PE 校验挡下。对 browser_download_url
	// 加这个头也无害（GitHub 照样直接回文件）。
	req.Header.Set("Accept", "application/octet-stream")
	resp, err := downloadClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	tmp := dest + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	total := resp.ContentLength
	var done int64
	buf := make([]byte, 64<<10)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				f.Close()
				os.Remove(tmp)
				return werr
			}
			done += int64(n)
			if progress != nil {
				progress(done, total)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			f.Close()
			os.Remove(tmp)
			return readErr
		}
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	// 完整性校验：Content-Length 缺失或提前 EOF 时，截断的 exe 会被当成正式包
	// 装上去并重启，且旧 exe 已被覆盖、无法回滚。这里两道都查。
	if total > 0 && done != total {
		os.Remove(tmp)
		return fmt.Errorf("下载不完整：已收 %d / 共 %d 字节", done, total)
	}
	if done == 0 {
		os.Remove(tmp)
		return errors.New("下载内容为空")
	}
	// PE 头校验：至少要是个可执行文件，别把 404 页面存成 exe
	if err := checkPE(tmp); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dest)
}

// checkPE 校验可执行映像及全部段，拒绝仅有 PE 头的截断文件。
func checkPE(path string) error {
	f, err := pe.Open(path)
	if err != nil {
		return fmt.Errorf("安装包不是有效的 Windows 可执行文件：%w", err)
	}
	defer f.Close()
	if f.Machine != pe.IMAGE_FILE_MACHINE_AMD64 || f.OptionalHeader == nil || len(f.Sections) == 0 || f.Characteristics&pe.IMAGE_FILE_EXECUTABLE_IMAGE == 0 {
		return errors.New("安装包不是完整的 Windows x64 程序")
	}
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	for _, section := range f.Sections {
		if section.Size > 0 && int64(section.Offset)+int64(section.Size) > st.Size() {
			return errors.New("安装包内容不完整，请重新下载")
		}
	}
	return nil
}

// Apply 把已下载的新 exe 换上并重启。当前进程应立即退出。
//
// 做法：写一个分离的 .bat，等 1.5s（让本进程释放 exe 句柄）后复制覆盖并重启。
// TempPath 新版本的临时落地路径
func TempPath(version string) string {
	return filepath.Join(config.BaseDir(), ".updates", fmt.Sprintf("zshtrans-%s.exe", strings.NewReplacer("/", "_", "\\", "_", ":", "_").Replace(version)))
}
