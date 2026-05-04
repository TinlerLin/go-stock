package agent

import (
	"context"
	"fmt"
	"go-stock/backend/data"
	"go-stock/backend/logger"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino-ext/components/model/claude"
	"github.com/cloudwego/eino-ext/components/model/deepseek"
	"github.com/cloudwego/eino-ext/components/model/gemini"
	"github.com/cloudwego/eino-ext/components/model/ollama"
	einoopenai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino-ext/components/model/openrouter"
	"github.com/cloudwego/eino-ext/components/model/qianfan"
	"github.com/cloudwego/eino-ext/components/model/qwen"
	"github.com/cloudwego/eino/components/model"
	"github.com/duke-git/lancet/v2/strutil"
	ollamaapi "github.com/eino-contrib/ollama/api"
	"google.golang.org/genai"
)

type chatModelProvider int

const (
	providerOpenAICompatible chatModelProvider = iota
	providerVolcArk
	providerDashScope
	providerOpenRouter
	providerAnthropic
	providerQianfan
	providerOllama
	providerGemini
	providerDeepSeek
)

func normalizeBaseURL(base string) string {
	return strings.TrimSuffix(strings.TrimSpace(base), "/")
}

func detectChatModelProvider(baseLower, modelName string) chatModelProvider {
	modelLower := strings.ToLower(strings.TrimSpace(modelName))

	if strings.Contains(baseLower, "volces.com") && strings.Contains(baseLower, "ark") {
		return providerVolcArk
	}
	if strings.Contains(baseLower, "dashscope.aliyuncs.com") ||
		strings.Contains(baseLower, "dashscope-intl.aliyuncs.com") {
		return providerDashScope
	}
	if strings.Contains(baseLower, "openrouter.ai") {
		return providerOpenRouter
	}
	if strings.Contains(baseLower, "anthropic.com") || strings.Contains(baseLower, "api.anthropic") {
		return providerAnthropic
	}
	if strings.Contains(baseLower, "qianfan.baidubce.com") || strings.Contains(baseLower, "aip.baidubce.com") {
		return providerQianfan
	}
	if strings.Contains(baseLower, ":11434") || strings.Contains(baseLower, "ollama") {
		return providerOllama
	}
	if isGeminiGoogleAI(baseLower, modelLower) {
		return providerGemini
	}
	if strings.Contains(baseLower, "api.deepseek.com") || strutil.ContainsAny(modelName, []string{"deepseek"}) {
		return providerDeepSeek
	}
	return providerOpenAICompatible
}

func isGeminiGoogleAI(baseLower, modelLower string) bool {
	if strings.Contains(baseLower, "generativelanguage.googleapis.com") ||
		strings.Contains(baseLower, "ai.google.dev") {
		return true
	}
	if strings.HasPrefix(modelLower, "gemini-") || strings.HasPrefix(modelLower, "gemini/") ||
		strings.HasPrefix(modelLower, "models/gemini") {
		return baseLower == "" || strings.Contains(baseLower, "googleapis.com")
	}
	return false
}

func parseAccessSecret(apiKey string) (ak, sk string) {
	s := strings.TrimSpace(apiKey)
	if s == "" {
		return "", ""
	}
	for _, sep := range []string{"|", ";"} {
		if idx := strings.Index(s, sep); idx > 0 && idx < len(s)-len(sep) {
			return strings.TrimSpace(s[:idx]), strings.TrimSpace(s[idx+len(sep):])
		}
	}
	if idx := strings.Index(s, ":"); idx > 0 && idx < len(s)-1 {
		// 避免误拆 http(s)://
		prefix := strings.ToLower(s[:idx])
		if strings.HasPrefix(prefix, "http") {
			return s, ""
		}
		return strings.TrimSpace(s[:idx]), strings.TrimSpace(s[idx+1:])
	}
	return s, ""
}

func ptrFloat32(v float32) *float32 { return &v }
func ptrBool(v bool) *bool          { return &v }

// CreateChatModel 按 Eino 生态组件路由（参见 https://www.cloudwego.io/zh/docs/eino/ecosystem_integration/chat_model/ ）
// 未命中专用实现时回退到 OpenAI 兼容 ChatModel（硅基流动、LM Studio、Azure OpenAI 等）。
func CreateChatModel(ctx context.Context, aiConfig data.AIConfig) (model.ToolCallingChatModel, error) {
	baseLower := strings.ToLower(normalizeBaseURL(aiConfig.BaseUrl))
	temperature := float32(aiConfig.Temperature)
	timeout := time.Duration(aiConfig.TimeOut) * time.Second
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	maxTok := aiConfig.MaxTokens
	if maxTok <= 0 {
		maxTok = 4096
	}

	p := detectChatModelProvider(baseLower, aiConfig.ModelName)
	logger.SugaredLogger.Infof("CreateChatModel provider=%d base=%q model=%q", p, aiConfig.BaseUrl, aiConfig.ModelName)

	switch p {
	case providerVolcArk:
		var thinking *ark.Thinking
		if aiConfig.Thinking {
			thinking = &ark.Thinking{Type: "enabled"}
		}
		return ark.NewChatModel(ctx, &ark.ChatModelConfig{
			BaseURL:     strings.TrimSpace(aiConfig.BaseUrl),
			Model:       aiConfig.ModelName,
			APIKey:      aiConfig.ApiKey,
			MaxTokens:   &aiConfig.MaxTokens,
			Temperature: &temperature,
			Thinking:    thinking,
		})

	case providerDashScope:
		cfg := &qwen.ChatModelConfig{
			APIKey:    aiConfig.ApiKey,
			BaseURL:   strings.TrimSpace(aiConfig.BaseUrl),
			Model:     aiConfig.ModelName,
			Timeout:   timeout,
			MaxTokens: &maxTok,
		}
		if aiConfig.Temperature > 0 {
			cfg.Temperature = ptrFloat32(temperature)
		}
		if aiConfig.Thinking {
			cfg.EnableThinking = ptrBool(true)
		}
		return qwen.NewChatModel(ctx, cfg)

	case providerOpenRouter:
		cfg := &openrouter.Config{
			APIKey:    aiConfig.ApiKey,
			BaseURL:   strings.TrimSpace(aiConfig.BaseUrl),
			Model:     aiConfig.ModelName,
			Timeout:   timeout,
			MaxTokens: &maxTok,
		}
		if aiConfig.Temperature > 0 {
			cfg.Temperature = ptrFloat32(temperature)
		}
		if aiConfig.Thinking {
			enabled := true
			cfg.Reasoning = &openrouter.Reasoning{Enabled: &enabled}
		}
		return openrouter.NewChatModel(ctx, cfg)

	case providerAnthropic:
		maxOut := aiConfig.MaxTokens
		if maxOut <= 0 {
			maxOut = 8192
		}
		cfg := &claude.Config{
			APIKey:    aiConfig.ApiKey,
			Model:     aiConfig.ModelName,
			MaxTokens: maxOut,
		}
		if aiConfig.Temperature > 0 {
			cfg.Temperature = ptrFloat32(temperature)
		}
		if b := strings.TrimSpace(aiConfig.BaseUrl); b != "" {
			cfg.BaseURL = &b
		}
		if aiConfig.Thinking {
			cfg.Thinking = &claude.Thinking{Enable: true, BudgetTokens: min(maxOut, 32000)}
		}
		return claude.NewChatModel(ctx, cfg)

	case providerQianfan:
		ak, sk := parseAccessSecret(aiConfig.ApiKey)
		if ak == "" || sk == "" {
			return nil, fmt.Errorf("千帆模型需在 API Key 中配置「AccessKey|SecretKey」（竖线或分号分隔），或参考 eino-ext qianfan 文档设置环境变量")
		}
		qcfg := qianfan.GetQianfanSingletonConfig()
		qcfg.AccessKey = ak
		qcfg.SecretKey = sk
		tok := maxTok
		cfg := &qianfan.ChatModelConfig{
			Model:               aiConfig.ModelName,
			MaxCompletionTokens: &tok,
		}
		if aiConfig.Temperature > 0 {
			cfg.Temperature = ptrFloat32(temperature)
		}
		return qianfan.NewChatModel(ctx, cfg)

	case providerOllama:
		base := strings.TrimSpace(aiConfig.BaseUrl)
		if base == "" {
			base = "http://127.0.0.1:11434"
		}
		opt := &ollamaapi.Options{}
		if aiConfig.Temperature > 0 {
			opt.Temperature = temperature
		}
		cfg := &ollama.ChatModelConfig{
			BaseURL: base,
			Model:   aiConfig.ModelName,
			Timeout: timeout,
			Options: opt,
		}
		if aiConfig.Thinking {
			tv := ollamaapi.ThinkValue{Value: true}
			cfg.Thinking = &tv
		}
		return ollama.NewChatModel(ctx, cfg)

	case providerGemini:
		cc := &genai.ClientConfig{APIKey: aiConfig.ApiKey}
		if b := strings.TrimSpace(aiConfig.BaseUrl); b != "" {
			cc.HTTPOptions = genai.HTTPOptions{BaseURL: b}
		}
		client, err := genai.NewClient(ctx, cc)
		if err != nil {
			return nil, fmt.Errorf("gemini genai client: %w", err)
		}
		gcfg := &gemini.Config{
			Client: client,
			Model:  aiConfig.ModelName,
		}
		if maxTok > 0 {
			gcfg.MaxTokens = &maxTok
		}
		if aiConfig.Temperature > 0 {
			gcfg.Temperature = ptrFloat32(temperature)
		}
		if aiConfig.Thinking {
			gcfg.ThinkingConfig = &genai.ThinkingConfig{IncludeThoughts: true}
		}
		return gemini.NewChatModel(ctx, gcfg)

	case providerDeepSeek:
		return deepseek.NewChatModel(ctx, &deepseek.ChatModelConfig{
			BaseURL:     strings.TrimSpace(aiConfig.BaseUrl),
			Model:       aiConfig.ModelName,
			APIKey:      aiConfig.ApiKey,
			MaxTokens:   maxTok,
			Temperature: temperature,
			Timeout:     timeout,
		})

	default:
		extraFields := map[string]any{}
		if aiConfig.Thinking {
			extraFields["thinking"] = map[string]any{"type": "enabled"}
		}
		mt := aiConfig.MaxTokens
		return einoopenai.NewChatModel(ctx, &einoopenai.ChatModelConfig{
			BaseURL:     strings.TrimSpace(aiConfig.BaseUrl),
			Model:       aiConfig.ModelName,
			APIKey:      aiConfig.ApiKey,
			Timeout:     timeout,
			MaxTokens:   &mt,
			Temperature: &temperature,
			ExtraFields: extraFields,
		})
	}
}
