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

	// Build system prompt blocks. Cache the single system block (trivially the
	// last) so the system + tool prefix is reused across turns and tool loops.
	var systemBlocks []anthropic.TextBlockParam
	if req.SystemPrompt != "" {
		systemBlocks = []anthropic.TextBlockParam{
			{Text: req.SystemPrompt, CacheControl: anthropic.NewCacheControlEphemeralParam()},
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

		// Recompute cache breakpoints on the (mutated, growing) messages slice
		// every iteration so stale markers never accumulate past the 4-breakpoint
		// budget (system + last tool + message writer + message reader).
		applyMessageCacheBreakpoints(messages)

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

		// Stop before executing tools if the request was cancelled mid-turn
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Surface each tool call to the frontend before executing it
		for _, tu := range toolUses {
			onChunk(StreamEvent{Type: "tool_use", ToolID: tu.ID, ToolName: tu.Name, Content: compactJSON(tu.Input)})
		}

		// Append the assistant message (with tool_use blocks) and tool results
		messages = append(messages, assistantMessageParam(&msg))
		messages = append(messages, executeToolsAndBuildResult(p.k8sClient, toolUses, onChunk))
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
		// No SDK constant for Sonnet 5 in v1.26.0; anthropic.Model is a plain string.
		return anthropic.Model("claude-sonnet-5")
	case "opus":
		// No SDK constant for Opus 4.8 in v1.26.0; anthropic.Model is a plain string.
		return anthropic.Model("claude-opus-4-8")
	case "haiku":
		return anthropic.ModelClaudeHaiku4_5
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

// applyMessageCacheBreakpoints clears every cache_control marker from the message
// content blocks, then re-marks the cascading writer/reader pair:
//   - writer at index len-2 (when len >= 3)
//   - reader at index len-4 (when len >= 5)
//
// Each tool-loop iteration (and each new turn) appends two messages, so the marker
// written at len-2 this call sits at len-4 next call: its prefix hash matches and
// reads at the cheap cache-read rate, while the new writer extends the cache. The
// clearing pass is required because SendMessage mutates and re-sends the same slice
// across iterations — without it, markers would accumulate past Anthropic's hard
// limit of 4 breakpoints and the API would reject the request.
func applyMessageCacheBreakpoints(messages []anthropic.MessageParam) {
	// Clearing pass: reset every cache-capable block to a zero-value marker. The
	// accessor returns a non-nil pointer for every variant that supports caching
	// (nil for thinking / redacted-thinking), so this covers all supported types.
	for mi := range messages {
		for bi := range messages[mi].Content {
			if p := messages[mi].Content[bi].GetCacheControl(); p != nil {
				*p = anthropic.CacheControlEphemeralParam{}
			}
		}
	}

	n := len(messages)
	if n >= 3 {
		markMessageCacheBreakpoint(messages[n-2])
	}
	if n >= 5 {
		markMessageCacheBreakpoint(messages[n-4])
	}
}

// markMessageCacheBreakpoint sets an ephemeral cache_control marker on the last
// cache-capable content block of the message, walking backwards. Assistant messages
// appended via ToParam() may end in a block type that does not accept cache_control
// (e.g. thinking); marking the last supported block still caches the whole message
// prefix. If no block supports caching, the breakpoint is silently skipped — a benign
// missed cache hit, not an error.
func markMessageCacheBreakpoint(msg anthropic.MessageParam) {
	for bi := len(msg.Content) - 1; bi >= 0; bi-- {
		if p := msg.Content[bi].GetCacheControl(); p != nil {
			*p = anthropic.NewCacheControlEphemeralParam()
			return
		}
	}
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
// Prices as of Jul 2026. Falls back to Sonnet pricing for unknown models.
var pricingTable = map[string]modelPricing{
	"claude-opus": {
		InputPerMillion:      5.0,
		OutputPerMillion:     25.0,
		CacheReadPerMillion:  0.5,
		CacheWritePerMillion: 6.25,
	},
	"claude-sonnet": {
		InputPerMillion:      3.0,
		OutputPerMillion:     15.0,
		CacheReadPerMillion:  0.3,
		CacheWritePerMillion: 3.75,
	},
	"claude-haiku": {
		InputPerMillion:      1.0,
		OutputPerMillion:     5.0,
		CacheReadPerMillion:  0.1,
		CacheWritePerMillion: 1.25,
	},
}

// estimateCost computes an estimated USD cost from token counts and model. It also
// populates the per-component cost fields on usage; the components sum to the returned
// total.
func estimateCost(usage *TokenUsage, model anthropic.Model) float64 {
	pricing := lookupPricing(string(model))
	usage.InputCostUSD = float64(usage.InputTokens) * pricing.InputPerMillion / 1_000_000
	usage.OutputCostUSD = float64(usage.OutputTokens) * pricing.OutputPerMillion / 1_000_000
	usage.CacheReadCostUSD = float64(usage.CacheReadTokens) * pricing.CacheReadPerMillion / 1_000_000
	usage.CacheWriteCostUSD = float64(usage.CacheCreationTokens) * pricing.CacheWritePerMillion / 1_000_000
	return usage.InputCostUSD + usage.OutputCostUSD + usage.CacheReadCostUSD + usage.CacheWriteCostUSD
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
