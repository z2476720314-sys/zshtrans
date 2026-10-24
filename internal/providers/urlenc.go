package providers

import "net/url"

func urlEncode(s string) string { return url.QueryEscape(s) }
