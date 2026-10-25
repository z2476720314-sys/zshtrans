//go:build !windows

package update

import "errors"

func Apply(string) error { return errors.New("自动安装仅支持 Windows") }
