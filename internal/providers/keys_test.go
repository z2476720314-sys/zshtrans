package providers

import (
	"testing"

	"github.com/local/zshtrans/internal/config"
)

func TestCustomKeyResolutionKeepsEnvironmentPriority(t *testing.T) {
	keys := config.APIKeys{}
	keys.Set("custom-provider", "placeholder")
	p := config.Provider{KeyRef: "custom-provider", KeyEnv: "CUSTOM_PROVIDER_KEY"}
	if ResolveKey(p, keys, func(string) string { return "example" }) != "example" {
		t.Fatal("configured environment key must retain priority")
	}
	if ResolveKey(p, keys, func(string) string { return "" }) != "placeholder" {
		t.Fatal("empty environment key must fall back to saved custom slot")
	}
	if ResolveKey(config.Provider{KeyRef: "acctoken"}, config.APIKeys{}, func(name string) string {
		if name == "ACCTOKEN_API_KEY" {
			return "example"
		}
		return ""
	}) != "example" {
		t.Fatal("legacy Acctoken environment fallback changed")
	}
}
