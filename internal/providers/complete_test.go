package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/local/zshtrans/internal/config"
)

func TestCancelTranslationCancelsHTTPRequest(t *testing.T) {
	started, stopped := make(chan struct{}), make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(started)
		<-r.Context().Done()
		close(stopped)
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := TranslateWithContext(ctx, config.Provider{BaseURL: srv.URL}, config.Model{ID: "test-model"}, "", "Привет", "ru", "zh")
		done <- err
	}()
	<-started
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel returned %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("translation ignored cancellation")
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("upstream request remained active")
	}
}

func TestTranslateStreamProtocols(t *testing.T) {
	for _, tc := range []struct{ protocol, body string }{
		{"openai-chat", "data: {\"choices\":[{\"delta\":{\"content\":\"你好\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"},
		{"openai-responses", "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"你好\"}\n\nevent: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[]}}\n\n"},
		{"anthropic-messages", "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"你好\"}}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"},
	} {
		t.Run(tc.protocol, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body["stream"] != true {
					t.Error("stream setting was not sent")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, tc.body)
			}))
			defer srv.Close()
			p := config.Provider{BaseURL: srv.URL, Protocol: tc.protocol, Stream: true}
			out, err := TranslateWith(p, config.Model{ID: "test-model"}, "", "Привет", "ru", "zh")
			if err != nil || out != "你好" {
				t.Fatalf("translation = %q, %v", out, err)
			}
		})
	}
}

func TestRejectIncompleteCompletion(t *testing.T) {
	for _, body := range []string{
		`{"choices":[{"message":{"content":"半句"},"finish_reason":"length"}]}`,
		`{"content":[{"type":"text","text":"半句"}],"stop_reason":"max_tokens"}`,
		`{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[{"content":[{"type":"output_text","text":"半句"}]}]}`,
		"data: {\"choices\":[{\"delta\":{\"content\":\"半句\"}}]}\n\n",
		"data: {\"choices\":[{\"delta\":{\"content\":\"半句\"},\"finish_reason\":\"length\"}]}\n\ndata: [DONE]\n\n",
		"data: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"半句\"}}\n\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"max_tokens\"}}\n\ndata: {\"type\":\"message_stop\"}\n\n",
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"半句\"}\n\ndata: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\",\"error\":{\"message\":\"failed\"}}}\n\n",
	} {
		if out, err := ExtractText([]byte(body)); err == nil {
			t.Errorf("accepted incomplete output %q from %s", out, body)
		}
	}
}

func TestTranslateOutputLimitIsIndependentOfThinkingBudget(t *testing.T) {
	var model config.Model
	if err := json.Unmarshal([]byte(`{"id":"test-model","output_limit":1800,"max_output_tokens":2048,"thinking_budget":64}`), &model); err != nil {
		t.Fatal(err)
	}
	model = NormalizeModel(model)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["max_tokens"] != float64(1800) {
			t.Errorf("max_tokens = %v, want 1800", body["max_tokens"])
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"你好"},"finish_reason":"stop"}]}`)
	}))
	defer srv.Close()
	if _, err := TranslateWith(config.Provider{BaseURL: srv.URL}, model, "", "Привет", "ru", "zh"); err != nil {
		t.Fatal(err)
	}
}

func TestGenerationLimitRespectsSelectionAndCatalogue(t *testing.T) {
	for _, tc := range []struct {
		selected, cap, thinking, want int
		fail                          bool
	}{
		{0, 0, 0, 8192, false}, {0, 2048, 64, 2048, false},
		{1800, 2048, 64, 1800, false}, {3000, 2048, 64, 0, true},
	} {
		model := NormalizeModel(config.Model{OutputLimit: tc.selected, MaxOutputTokens: tc.cap, ThinkingBudget: tc.thinking})
		got, err := GenerationLimit(model)
		if got != tc.want || (err != nil) != tc.fail {
			t.Errorf("GenerationLimit(%+v) = %d,%v", tc, got, err)
		}
	}
}

func TestVisionStreamAndTruncation(t *testing.T) {
	for _, tc := range []struct {
		protocol, body string
		fail           bool
	}{
		{"openai-completions", "data: {\"choices\":[{\"delta\":{\"content\":\"ПВЗ 784512369\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", false},
		{"openai-responses", "data: {\"type\":\"response.output_text.delta\",\"delta\":\"ПВЗ 784512369\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n", false},
		{"anthropic-messages", "data: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"ПВЗ 784512369\"}}\n\ndata: {\"type\":\"message_stop\"}\n\n", false},
		{"openai-completions", `{"choices":[{"message":{"content":"ПВЗ 784512369"},"finish_reason":"length"}]}`, true},
	} {
		t.Run(tc.protocol+fmt.Sprint(tc.fail), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body["stream"] != true {
					t.Error("vision ignored stream flag")
				}
				fmt.Fprint(w, tc.body)
			}))
			defer srv.Close()
			out, err := VisionOCR(config.Provider{BaseURL: srv.URL, Protocol: tc.protocol, Stream: true}, config.Model{ID: "vision"}, "", "cG5n")
			if (err != nil) != tc.fail || (!tc.fail && out != "ПВЗ 784512369") {
				t.Fatalf("vision = %q,%v", out, err)
			}
		})
	}
}

func TestDirectLLMUsesTranslationContract(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct{ Role, Content string }
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if len(body.Messages) == 0 || !strings.Contains(body.Messages[0].Content, "ПВЗ") {
			t.Error("direct model bypassed the Russian translation contract")
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"译文：包裹还没到取货点，我还没取货。"},"finish_reason":"stop"}]}`)
	}))
	defer srv.Close()
	out, err := chatTranslate(context.Background(), srv.URL, "test-model", "", "Посылка до ПВЗ ещё не дошла. Я товар не выкупал.", "ru", "zh", nil)
	if err != nil || out != "包裹还没到取货点，我还没取货。" {
		t.Fatalf("translation = %q, %v", out, err)
	}
}
