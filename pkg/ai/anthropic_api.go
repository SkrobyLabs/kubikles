package ai

import (
	"context"
	"fmt"
	"log"

	"kubikles/pkg/k8s"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// AnthropicAPIProvider implements Provider using the Anthropic Messages API directly.
type AnthropicAPIProvider struct {
	k8sClient *k8s.Client
}

// NewAnthropicAPIProvider creates a new Anthropic API provider.
func NewAnthropicAPIProvider(k8sClient *k8s.Client) *AnthropicAPIProvider {
	return &AnthropicAPIProvider{k8sClient: k8sClient}
}

func (p *AnthropicAPIProvider) Name() string {
	return "Anthropic API"
}

func (p *AnthropicAPIProvider) IsAvailable() (bool, string) {
	status := GetAnthropicAPIKeyStatus()
	switch status {
	case "env":
		return true, "API key set via ANTHROPIC_API_KEY environment variable"
	case "configured":
		return true, "API key configured"
	default:
		return false, "API key not configured. Set ANTHROPIC_API_KEY or configure in Settings → AI."
	}
}

// SupportsSession returns false — the API is stateless.
// The manager uses sendMessageOneShot which accumulates history.
func (p *AnthropicAPIProvider) SupportsSession() bool {
	return false
}

// Capabilities returns the provider's capabilities.
func (p *AnthropicAPIProvider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		SupportsStreaming: true,
		SupportsSessions:  false,
		SupportedTools:    nil,    // all tools supported
		MaxContextLength:  200000, // Claude models support large context
	}
}

// StartSession is not supported by the API provider.
func (p *AnthropicAPIProvider) StartSession(_, _, _, _ string, _, _ []string, _ func(StreamEvent)) (Session, error) {
	return nil, fmt.Errorf("anthropic API does not support persistent sessions")
}

// SendMessage implements the Provider interface with a streaming tool-use loop.
func (p *AnthropicAPIProvider) SendMessage(ctx context.Context, req Request, onChunk func(StreamEvent)) error {
	apiKey := resolveAnthropicAPIKey()
	if apiKey == "" {
		return fmt.Errorf("anthropic API key not configured")
	}
	client := anthropic.NewClient(option.WithAPIKey(apiKey))

	// Configure command allowlist before executing any tools
	setAllowedCommands(req.AllowedCommands)

	resolvedModel := mapModel(req.Model)
	messages := convertHistory(req.History, req.Message)
	toolParams := buildToolParams(req.AllowedTools)

	// Build system prompt blocks
	var systemBlocks []anthropic.TextBlockParam
	if req.SystemPrompt != "" {
		systemBlocks = []anthropic.TextBlockParam{
			{Text: req.SystemPrompt},
		}
	}

	var totalUsage TokenUsage
	const maxToolLoops = 25 // safety limit to prevent infinite tool loops

	for i := 0; i < maxToolLoops; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		params := anthropic.MessageNewParams{
			Model:     resolvedModel,
			MaxTokens: 16384,
			Messages:  messages,
			System:    systemBlocks,
		}
		if len(toolParams) > 0 {
			params.Tools = toolParams
		}

		stream := client.Messages.NewStreaming(ctx, params)

		msg := anthropic.Message{}
		for stream.Next() {
			event := stream.Current()
			if err := msg.Accumulate(event); err != nil {
				stream.Close()
				return fmt.Errorf("stream accumulate error: %w", err)
			}

			// Emit text deltas as they arrive
			if evt, ok := event.AsAny().(anthropic.ContentBlockDeltaEvent); ok {
				if evt.Delta.Text != "" {
					onChunk(StreamEvent{Type: "text", Content: evt.Delta.Text})
				}
			}
		}

		if err := stream.Err(); err != nil {
			stream.Close()
			return fmt.Errorf("anthropic API stream error: %w", err)
		}
		stream.Close()

		accumulateUsage(&totalUsage, msg.Usage)

		toolUses := extractToolUses(&msg)
		if len(toolUses) == 0 {
			// No tool calls — conversation turn is done
			totalUsage.CostUSD = estimateCost(&totalUsage, resolvedModel)
			onChunk(StreamEvent{Type: "done", Usage: &totalUsage})
			return nil
		}

		// Execute tools and continue the loop
		log.Printf("[AI] Tool loop iteration %d: %d tool call(s)", i+1, len(toolUses))

		// Append the assistant message (with tool_use blocks) and tool results
		messages = append(messages, assistantMessageParam(&msg))
		messages = append(messages, executeToolsAndBuildResult(p.k8sClient, toolUses))
	}

	// Safety: exceeded max tool loops
	totalUsage.CostUSD = estimateCost(&totalUsage, resolvedModel)
	onChunk(StreamEvent{Type: "error", Content: "exceeded maximum tool use iterations"})
	onChunk(StreamEvent{Type: "done", Usage: &totalUsage})
	return nil
}

// mapModel maps short model aliases to full Anthropic model IDs.
func mapModel(name string) anthropic.Model {
	switch name {
	case "sonnet":
		return anthropic.ModelClaudeSonnet4_6
	case "opus":
		return anthropic.ModelClaudeOpus4_6
	case "haiku":
		return anthropic.ModelClaudeHaiku4_5_20251001
	default:
		// Passthrough for full model IDs
		return anthropic.Model(name)
	}
}

// convertHistory builds the Anthropic messages array from conversation history plus the new message.
func convertHistory(history []Message, newMessage string) []anthropic.MessageParam {
	var messages []anthropic.MessageParam
	for _, msg := range history {
		switch msg.Role {
		case "user":
			messages = append(messages, anthropic.NewUserMessage(
				anthropic.NewTextBlock(msg.Content),
			))
		case "assistant":
			messages = append(messages, anthropic.NewAssistantMessage(
				anthropic.NewTextBlock(msg.Content),
			))
		}
	}
	// Add the new user message
	messages = append(messages, anthropic.NewUserMessage(
		anthropic.NewTextBlock(newMessage),
	))
	return messages
}

// assistantMessageParam converts a response Message into a MessageParam preserving all content blocks.
func assistantMessageParam(msg *anthropic.Message) anthropic.MessageParam {
	return msg.ToParam()
}

// accumulateUsage adds usage from a single API call to the running total.
func accumulateUsage(total *TokenUsage, usage anthropic.Usage) {
	total.InputTokens += int(usage.InputTokens)
	total.OutputTokens += int(usage.OutputTokens)
	total.CacheReadTokens += int(usage.CacheReadInputTokens)
	total.CacheCreationTokens += int(usage.CacheCreationInputTokens)
}

// modelPricing holds per-million-token pricing for cost estimation.
type modelPricing struct {
	InputPerMillion      float64
	OutputPerMillion     float64
	CacheReadPerMillion  float64
	CacheWritePerMillion float64
}

// pricingTable maps model ID prefixes to pricing (USD per million tokens).
// Prices as of Feb 2026. Falls back to Sonnet pricing for unknown models.
var pricingTable = map[string]modelPricing{
	"claude-opus": {
		InputPerMillion:      15.0,
		OutputPerMillion:     75.0,
		CacheReadPerMillion:  1.5,
		CacheWritePerMillion: 18.75,
	},
	"claude-sonnet": {
		InputPerMillion:      3.0,
		OutputPerMillion:     15.0,
		CacheReadPerMillion:  0.3,
		CacheWritePerMillion: 3.75,
	},
	"claude-haiku": {
		InputPerMillion:      0.80,
		OutputPerMillion:     4.0,
		CacheReadPerMillion:  0.08,
		CacheWritePerMillion: 1.0,
	},
}

// estimateCost computes an estimated USD cost from token counts and model.
func estimateCost(usage *TokenUsage, model anthropic.Model) float64 {
	pricing := lookupPricing(string(model))
	cost := float64(usage.InputTokens)*pricing.InputPerMillion/1_000_000 +
		float64(usage.OutputTokens)*pricing.OutputPerMillion/1_000_000 +
		float64(usage.CacheReadTokens)*pricing.CacheReadPerMillion/1_000_000 +
		float64(usage.CacheCreationTokens)*pricing.CacheWritePerMillion/1_000_000
	return cost
}

func lookupPricing(model string) modelPricing {
	for prefix, p := range pricingTable {
		if len(model) >= len(prefix) && model[:len(prefix)] == prefix {
			return p
		}
	}
	// Default to Sonnet pricing for unknown models
	return pricingTable["claude-sonnet"]
}
