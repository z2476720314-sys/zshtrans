package config

import (
	"encoding/json"
	"testing"
)

func TestAPIKeysPreservesCustomSlotsAndDefaults(t *testing.T) {
	keys := APIKeys{OCRSpace: "helloworld"}
	if err := json.Unmarshal([]byte(`{"qwen":"placeholder","openai":"example","custom-provider":"placeholder","ocrspace":null}`), &keys); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(keys)
	if err != nil {
		t.Fatal(err)
	}
	var saved map[string]string
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"qwen": "placeholder", "openai": "example", "custom-provider": "placeholder", "ocrspace": "helloworld"} {
		if saved[name] != want {
			t.Errorf("credential slot %q did not survive JSON roundtrip", name)
		}
	}
}

func TestStoreGetDoesNotShareCustomKeys(t *testing.T) {
	var cfg Config
	if err := json.Unmarshal([]byte(`{"api_keys":{"custom-provider":"placeholder"}}`), &cfg); err != nil {
		t.Fatal(err)
	}
	store := NewStore(cfg)
	snapshot := store.Get()
	if err := json.Unmarshal([]byte(`{"custom-provider":"example"}`), &snapshot.APIKeys); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(store.Get().APIKeys)
	if err != nil {
		t.Fatal(err)
	}
	var saved map[string]string
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	if saved["custom-provider"] != "placeholder" {
		t.Fatal("mutating a returned config changed or lost the stored custom credential")
	}
}
