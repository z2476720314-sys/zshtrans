//go:build !windows

package deskwin

func ShowResult(Result) error { return ErrUnavailable }
func CloseResult()            {}
