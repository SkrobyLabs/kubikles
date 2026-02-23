package ai

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

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

// executeToolsAndBuildResult executes each tool call and returns a user message containing tool results.
func executeToolsAndBuildResult(k8sClient *k8s.Client, toolUses []anthropic.ToolUseBlock) anthropic.MessageParam {
	var resultBlocks []anthropic.ContentBlockParamUnion
	if k8sClient == nil {
		for _, tu := range toolUses {
			resultBlocks = append(resultBlocks,
				anthropic.NewToolResultBlock(tu.ID, "Kubernetes client not initialized — cannot execute tools", true),
			)
		}
		return anthropic.NewUserMessage(resultBlocks...)
	}
	for _, tu := range toolUses {
		// Parse tool input
		var args map[string]interface{}
		if err := json.Unmarshal(tu.Input, &args); err != nil {
			log.Printf("[AI] Failed to parse tool input for %s: %v", tu.Name, err)
			resultBlocks = append(resultBlocks,
				anthropic.NewToolResultBlock(tu.ID, fmt.Sprintf("error parsing tool input: %v", err), true),
			)
			continue
		}

		log.Printf("[AI] Executing tool: %s args=%v", tu.Name, args)
		result, isError := tools.CallTool(k8sClient, tu.Name, args)
		resultBlocks = append(resultBlocks,
			anthropic.NewToolResultBlock(tu.ID, result, isError),
		)
	}

	return anthropic.NewUserMessage(resultBlocks...)
}

// setAllowedCommands configures the command allowlist for the run_command tool.
func setAllowedCommands(allowedCommands []string) {
	// Set AllowedCommandPrefixes so run_command uses the configured allowlist.
	// Same pattern as the MCP server.
	tools.AllowedCommandPrefixes = allowedCommands
}
