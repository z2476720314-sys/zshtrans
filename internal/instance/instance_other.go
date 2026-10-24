//go:build !windows

package instance

// fileLock 非 Windows 平台不做单实例限制
type fileLock struct{}

func acquire(name string) (Lock, error) { return &fileLock{}, nil }

func (l *fileLock) Release() {}
