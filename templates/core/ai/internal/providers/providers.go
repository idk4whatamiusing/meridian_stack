// Package providers - LLM provider abstraction for GENERAL chat.
// cloudflare  -> Cloudflare Workers AI (@cf/meta/llama-3.1-8b-instruct, SSE)
// bedrock     -> Amazon Bedrock ConverseStream
// openai      -> any OpenAI-compatible endpoint (also used by local llama.cpp)
package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

type Message struct {
	Role    string `json:"role"` // system | user | assistant
	Content string `json:"content"`
}

type Chunk struct {
	Delta string
	Done  bool
	Model string
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// Provider returns the streaming chat implementation selected by AI_PROVIDER.
func Provider(ctx context.Context) (func([]Message) (<-chan Chunk, error), string) {
	switch strings.ToLower(envOr("AI_PROVIDER", "cloudflare")) {
	case "bedrock":
		return bedrockStream, "bedrock:" + envOr("BEDROCK_MODEL_ID", "anthropic.claude-3-haiku-20240307-v1:0")
	case "openai":
		return openaiStream(envOr("OPENAI_BASE_URL", "https://api.openai.com/v1"), os.Getenv("OPENAI_API_KEY"),
			envOr("OPENAI_MODEL", "gpt-4o-mini")), "openai"
	default:
		return cfStream, "cloudflare:" + envOr("CF_AI_MODEL", "@cf/meta/llama-3.1-8b-instruct")
	}
}

// OpenAIStream is exported so the support LLM (llama.cpp server, which speaks
// the OpenAI protocol) can reuse it.
func OpenAIStream(base, key, model string) func([]Message) (<-chan Chunk, error) {
	return openaiStream(base, key, model)
}

// ---- cloudflare workers ai (SSE: data: {"response":"tok"}) ----

type cfChunk struct {
	Response string `json:"response"`
}

func cfStream(msgs []Message) (<-chan Chunk, error) {
	account := os.Getenv("CF_ACCOUNT_ID")
	token := os.Getenv("CF_API_TOKEN")
	model := envOr("CF_AI_MODEL", "@cf/meta/llama-3.1-8b-instruct")
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/ai/run/%s", account, model)
	body, _ := json.Marshal(map[string]any{"messages": msgs, "stream": true})
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("workers ai %d: %s", resp.StatusCode, b)
	}
	out := make(chan Chunk, 32)
	go sseLoop(resp.Body, func(data []byte) {
		var c cfChunk
		if json.Unmarshal(data, &c) == nil && c.Response != "" {
			out <- Chunk{Delta: c.Response}
		}
	}, func() { close(out) })
	return out, nil
}

// shared SSE reader: emits each `data:` line then closes when [DONE]/EOF
func sseLoop(body io.ReadCloser, on dataFn, done func()) {
	defer body.Close()
	defer done()
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			return
		}
		on([]byte(payload))
	}
}

type dataFn func([]byte)

// ---- openai compatible (works for llama.cpp too) ----

type oaDelta struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

func openaiStream(base, key, model string) func([]Message) (<-chan Chunk, error) {
	return func(msgs []Message) (<-chan Chunk, error) {
		body, _ := json.Marshal(map[string]any{"model": model, "messages": msgs, "stream": true})
		req, _ := http.NewRequest(http.MethodPost, strings.TrimRight(base, "/")+"/chat/completions", bytes.NewReader(body))
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("llm %d: %s", resp.StatusCode, b)
		}
		out := make(chan Chunk, 32)
		go sseLoop(resp.Body, func(data []byte) {
			var c oaDelta
			if json.Unmarshal(data, &c) == nil && len(c.Choices) > 0 && c.Choices[0].Delta.Content != "" {
				out <- Chunk{Delta: c.Choices[0].Delta.Content}
			}
		}, func() { close(out) })
		return out, nil
	}
}

// ---- bedrock converse stream ----

var brClient *bedrockruntime.Client

func bedrock() *bedrockruntime.Client {
	if brClient == nil {
		cfg, err := awsconfig.LoadDefaultConfig(context.Background())
		if err != nil {
			panic(err)
		}
		brClient = bedrockruntime.NewFromConfig(cfg)
	}
	return brClient
}

func bedrockStream(msgs []Message) (<-chan Chunk, error) {
	model := envOr("BEDROCK_MODEL_ID", "anthropic.claude-3-haiku-20240307-v1:0")
	var sys []types.SystemContentBlock
	var conv []types.Message
	for _, m := range msgs {
		if m.Role == "system" {
			sys = append(sys, &types.SystemContentBlockMemberText{Value: m.Content})
			continue
		}
		role := types.ConversationRoleUser
		if m.Role == "assistant" {
			role = types.ConversationRoleAssistant
		}
		conv = append(conv, types.Message{
			Role: role,
			Content: []types.ContentBlock{
				&types.ContentBlockMemberText{Value: m.Content},
			},
		})
	}
	out, err := bedrock().ConverseStream(context.Background(), &bedrockruntime.ConverseStreamInput{
		ModelId:    aws.String(model),
		Messages:   conv,
		System:     sys,
		InferenceConfig: &types.InferenceConfiguration{MaxTokens: aws.Int32(1024)},
	})
	if err != nil {
		return nil, err
	}
	ch := make(chan Chunk, 32)
	go func() {
		defer close(ch)
		stream := out.GetStream()
		for {
			ev, ok := <-stream.Events()
			if !ok {
				return
			}
			switch v := ev.(type) {
			case *types.ConverseStreamOutputMemberContentBlockDelta:
				if d := v.Value.Delta; d != nil {
					if t, ok := d.(*types.ContentBlockDeltaMemberText); ok && t.Value != "" {
						ch <- Chunk{Delta: t.Value}
					}
				}
			case *types.ConverseStreamOutputMemberMetadata:
				ch <- Chunk{Done: true}
				return
			case error:
				ch <- Chunk{Done: true}
				return
			}
		}
	}()
	return ch, nil
}
