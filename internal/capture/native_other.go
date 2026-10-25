//go:build !windows

package capture

import "context"

func Interactive(context.Context) (CaptureResult, error) { return CaptureResult{}, errUnsupported }
func CloseInteractive() error                            { return nil }
