package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"unicode/utf8"

	"kubikles/pkg/k8s"
	"kubikles/pkg/tools"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
)

// buildToolParams converts the allowed ToolDefs into Anthropic API ToolUnionParam format.
func buildToolParams(allowedTools []string) []anthropic.ToolUnionParam {
	allowed := makeAllowSet(allowedTools)

	var params []anthropic.ToolUnionParam
	for _, td := range tools.AllToolDefs() {
		if !allowed[td.Name] {
			continue
		}

		schema := td.InputSchema.(map[string]interface{})
		tp := anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        td.Name,
				Description: param.NewOpt(td.Description),
				InputSchema: buildInputSchema(schema),
			},
		}
		params = append(params, tp)
	}
	// Cache the tool definitions as part of the stable prefix (tools precede the
	// system prompt in the prefix hash). Marking only the last tool caches all of
	// them as one block.
	if len(params) > 0 {
		params[len(params)-1].OfTool.CacheControl = anthropic.NewCacheControlEphemeralParam()
	}
	return params
}

// buildInputSchema converts a map-based JSON schema into ToolInputSchemaParam.
func buildInputSchema(schema map[string]interface{}) anthropic.ToolInputSchemaParam {
	result := anthropic.ToolInputSchemaParam{
		Type: "object",
	}
	if props, ok := schema["properties"]; ok {
		result.Properties = props
	}
	if req, ok := schema["required"].([]string); ok {
		result.Required = req
	}
	return result
}

// makeAllowSet builds a set of short tool names from fully-qualified names.
// e.g. "mcp__kubikles__get_pod_logs" → "get_pod_logs"
func makeAllowSet(allowedTools []string) map[string]bool {
	cfg := GetConfig()
	set := make(map[string]bool, len(allowedTools))
	for _, t := range allowedTools {
		if strings.HasPrefix(t, cfg.MCPPrefix) {
			set[strings.TrimPrefix(t, cfg.MCPPrefix)] = true
		} else {
			// Also allow bare tool names (from direct API usage)
			set[t] = true
		}
	}
	return set
}

// extractToolUses returns the tool_use content blocks from a completed message.
func extractToolUses(msg *anthropic.Message) []anthropic.ToolUseBlock {
	var uses []anthropic.ToolUseBlock
	for _, block := range msg.Content {
		if tu, ok := block.AsAny().(anthropic.ToolUseBlock); ok {
			uses = append(uses, tu)
		}
	}
	return uses
}

// toolResultTruncateLimit caps the tool result text forwarded to the UI.
const toolResultTruncateLimit = 2000

// compactJSON removes insignificant whitespace from a JSON payload for a compact
// single-line representation. Returns the raw string if compaction fails.
func compactJSON(raw []byte) string {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return string(raw)
	}
	return buf.String()
}

// truncateForUI shortens s to at most max bytes, appending a marker when cut.
// The cut is backed up to a UTF-8 rune boundary so a multibyte character is
// never split (which would render as a replacement glyph in the UI).
func truncateForUI(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "… [truncated]"
}

// executeToolsAndBuildResult executes each tool call and returns a user message containing tool results.
// onEvent (may be nil) receives a tool_result StreamEvent per tool right after execution.
func executeToolsAndBuildResult(k8sClient *k8s.Client, toolUses []anthropic.ToolUseBlock, onEvent func(StreamEvent)) anthropic.MessageParam {
	emitResult := func(tu anthropic.ToolUseBlock, result string, isError bool) {
		if onEvent != nil {
			onEvent(StreamEvent{
				Type:     "tool_result",
				ToolID:   tu.ID,
				ToolName: tu.Name,
				Content:  truncateForUI(result, toolResultTruncateLimit),
				IsError:  isError,
			})
		}
	}

	var resultBlocks []anthropic.ContentBlockParamUnion
	if k8sClient == nil {
		for _, tu := range toolUses {
			const msg = "Kubernetes client not initialized — cannot execute tools"
			resultBlocks = append(resultBlocks,
				anthropic.NewToolResultBlock(tu.ID, msg, true),
			)
			emitResult(tu, msg, true)
		}
		return anthropic.NewUserMessage(resultBlocks...)
	}
	for _, tu := range toolUses {
		// Parse tool input
		var args map[string]interface{}
		if err := json.Unmarshal(tu.Input, &args); err != nil {
			log.Printf("[AI] Failed to parse tool input for %s: %v", tu.Name, err)
			errMsg := fmt.Sprintf("error parsing tool input: %v", err)
			resultBlocks = append(resultBlocks,
				anthropic.NewToolResultBlock(tu.ID, errMsg, true),
			)
			emitResult(tu, errMsg, true)
			continue
		}

		log.Printf("[AI] Executing tool: %s args=%v", tu.Name, args)
		result, isError := tools.CallTool(k8sClient, tu.Name, args)
		resultBlocks = append(resultBlocks,
			anthropic.NewToolResultBlock(tu.ID, result, isError),
		)
		emitResult(tu, result, isError)
	}

	return anthropic.NewUserMessage(resultBlocks...)
}

// setAllowedCommands configures the command allowlist for the run_command tool.
func setAllowedCommands(allowedCommands []string) {
	// Set AllowedCommandPrefixes so run_command uses the configured allowlist.
	// Same pattern as the MCP server.
	tools.AllowedCommandPrefixes = allowedCommands
}
