package capture

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const qqVersion = "9.9.30-48517"
const captureIdleGrace = 1200 * time.Millisecond

//go:embed bundled/qq-runtime.zip
var runtimeArchive []byte

var ErrCanceled = errors.New("截图已取消")
var ErrBusy = errors.New("截图正在进行中")
var runtimeFilesMu sync.Mutex

type CaptureResult struct {
	PNG    []byte
	Action string
}

type nativeManifest struct {
	Version string            `json:"qq_version"`
	Files   map[string]string `json:"files"`
}

func safeRuntimePath(name string) bool {
	name = strings.ReplaceAll(name, "\\", "/")
	if name == "" || strings.Contains(name, ":") || strings.HasPrefix(name, "/") {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." || part == "." || part == "" {
			return false
		}
	}
	return filepath.IsLocal(filepath.FromSlash(name))
}

func bundledManifest() (nativeManifest, *zip.Reader, error) {
	r, err := zip.NewReader(bytes.NewReader(runtimeArchive), int64(len(runtimeArchive)))
	if err != nil {
		return nativeManifest{}, nil, err
	}
	f, err := r.Open("native_manifest.json")
	if err != nil {
		return nativeManifest{}, nil, err
	}
	defer f.Close()
	var manifest nativeManifest
	if err := json.NewDecoder(f).Decode(&manifest); err != nil {
		return manifest, nil, err
	}
	if manifest.Version != qqVersion || len(manifest.Files) != 71 {
		return manifest, nil, errors.New("原生截图清单版本不匹配")
	}
	return manifest, r, nil
}

func validateManifestFiles(base string, manifest nativeManifest) error {
	for name, expected := range manifest.Files {
		if !safeRuntimePath(name) {
			return errors.New("原生截图清单路径无效")
		}
		file, err := os.Open(filepath.Join(base, filepath.FromSlash(name)))
		if err != nil {
			return fmt.Errorf("原生截图文件缺失 %s: %w", filepath.Base(name), err)
		}
		hash := sha256.New()
		_, err = io.Copy(hash, file)
		closeErr := file.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		if !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), expected) {
			return fmt.Errorf("原生截图文件校验失败: %s", filepath.Base(name))
		}
	}
	return nil
}

// EnsureRuntime releases the bundled, hash-verified files next to application data.
func EnsureRuntime(base string) (string, error) {
	runtimeFilesMu.Lock()
	defer runtimeFilesMu.Unlock()
	manifest, archive, err := bundledManifest()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "native-runtime", qqVersion)
	if _, err := os.Stat(dir); err == nil {
		return dir, validateManifestFiles(dir, manifest)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0700); err != nil {
		return "", err
	}
	stage, err := os.MkdirTemp(filepath.Dir(dir), ".extract-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	for _, entry := range archive.File {
		if !safeRuntimePath(entry.Name) {
			return "", errors.New("原生截图压缩包路径无效")
		}
		if _, ok := manifest.Files[entry.Name]; !ok && entry.Name != "native_manifest.json" && entry.Name != "QQ_CAPTURE_LICENSE" {
			return "", errors.New("原生截图压缩包包含未登记文件")
		}
		path := filepath.Join(stage, filepath.FromSlash(entry.Name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return "", err
		}
		if err := extractRuntimeFile(entry, path); err != nil {
			return "", err
		}
	}
	if err := validateManifestFiles(stage, manifest); err != nil {
		return "", err
	}
	if err := os.Rename(stage, dir); err != nil {
		return "", err
	}
	return dir, nil
}

func extractRuntimeFile(entry *zip.File, path string) error {
	source, err := entry.Open()
	if err != nil {
		return err
	}
	defer source.Close()
	target, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(target, source)
	closeErr := target.Close()
	return errors.Join(copyErr, closeErr)
}

func readCaptureImage(payload []byte) ([]byte, error) {
	path := string(bytes.TrimRight(payload, "\x00"))
	if !filepath.IsAbs(path) || strings.ContainsRune(path, 0) || strings.HasPrefix(path, "\\\\") {
		return nil, errors.New("原生截图没有返回有效的本地图片路径")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > 64<<20 {
		return nil, errors.New("截图文件过大")
	}
	config, _, err := image.DecodeConfig(file)
	if err != nil {
		return nil, err
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 40_000_000 {
		return nil, errors.New("截图像素尺寸过大")
	}
	if _, err := file.Seek(0, 0); err != nil {
		return nil, err
	}
	decoded, _, err := image.Decode(file)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := png.Encode(&out, decoded); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

type nativeEvent struct {
	Name    string
	Payload []byte
}
type captureState struct {
	Action  string
	Payload []byte
	idleAt  time.Time
	Err     error
}

func (s *captureState) accept(event nativeEvent, now time.Time) {
	switch event.Name {
	case "IPC_DISCONNECTED", "IPC_PAYLOAD_ERROR":
		s.Err = errors.New("原生截图连接中断或回调无效")
	case "onScreenShotRequestTranslate", "onScreenShotRequestOCR":
		if s.Action != "translate" && s.Action != "ocr" {
			s.Action = "ocr"
			if event.Name == "onScreenShotRequestTranslate" {
				s.Action = "translate"
			}
			s.Payload = event.Payload
		}
		s.idleAt = now
	case "onScreenShotCancel", "onScreenShotComplete", "onScreenShotShare", "onScreenShotLongCaptureOk", "onScreenShotLongCaptureSave", "onScreenShotLongCaptureCollect", "onScreenShotLongCaptureTransfer", "onScreenShotLongCaptureSendToMobile", "onScreenShotLongCaptureSaveToEmoj":
		if s.Action == "" {
			s.Action = "complete"
			if event.Name == "onScreenShotCancel" {
				s.Action = "cancel"
			}
		}
		s.idleAt = now
	}
}

func (s *captureState) ready(now time.Time) bool {
	return !s.idleAt.IsZero() && now.Sub(s.idleAt) >= captureIdleGrace
}

func repairedInstruction(before []byte) ([]byte, error) {
	original := []byte{0x48, 0x8d, 0x4d, 0xe0}
	repaired := []byte{0x48, 0x8d, 0x4d, 0xe8}
	if !bytes.Equal(before, original) && !bytes.Equal(before, repaired) {
		return nil, errors.New("原生工具栏指令不匹配")
	}
	return repaired, nil
}

func copyWhenReleased(ctx context.Context, pressed func() bool, copy func() error) error {
	ticker := time.NewTicker(15 * time.Millisecond)
	defer ticker.Stop()
	for pressed() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return copy()
}
