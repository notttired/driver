package pi

import (
	"encoding/json"
	"strings"
)

// ToolCall is one `toolCall` content block of an assistant message.
type ToolCall struct {
	ID      string
	Name    string
	Command string // arguments.command, for bash
}

// ToolResult is one `toolResult` message: the text of its content blocks.
type ToolResult struct {
	ToolCallID string
	ToolName   string
	Text       string
	IsError    bool
}

// Entry is the part of one session JSONL line driver cares about. For a
// message entry one of Text (user), ToolCalls (assistant) or Result
// (toolResult) is set; every other entry type (session, model_change, usage,
// compaction, …) parses to Kind != "message" and is skipped by callers.
type Entry struct {
	Kind      string // the line's "type"
	Role      string // message.role: user | assistant | toolResult
	Text      string // user: the message's text blocks, newline-joined
	ToolCalls []ToolCall
	Result    *ToolResult
}

type rawEntry struct {
	Type    string `json:"type"`
	Message *struct {
		Role       string          `json:"role"`
		ToolCallID string          `json:"toolCallId"`
		ToolName   string          `json:"toolName"`
		IsError    bool            `json:"isError"`
		Content    json.RawMessage `json:"content"`
	} `json:"message"`
}

type rawBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ParseEntry decodes one JSONL line. ok is false for lines that are not
// JSON objects with a "type".
func ParseEntry(line []byte) (e Entry, ok bool) {
	var raw rawEntry
	if err := json.Unmarshal(line, &raw); err != nil || raw.Type == "" {
		return Entry{}, false
	}
	e.Kind = raw.Type
	if raw.Type != "message" || raw.Message == nil {
		return e, true
	}
	e.Role = raw.Message.Role
	blocks := parseBlocks(raw.Message.Content)
	switch raw.Message.Role {
	case "user":
		e.Text = textOf(blocks)
	case "assistant":
		for _, b := range blocks {
			if b.Type != "toolCall" {
				continue
			}
			tc := ToolCall{ID: b.ID, Name: b.Name}
			if len(b.Arguments) > 0 {
				var args struct {
					Command string `json:"command"`
				}
				_ = json.Unmarshal(b.Arguments, &args)
				tc.Command = args.Command
			}
			e.ToolCalls = append(e.ToolCalls, tc)
		}
	case "toolResult":
		e.Result = &ToolResult{
			ToolCallID: raw.Message.ToolCallID,
			ToolName:   raw.Message.ToolName,
			Text:       textOf(blocks),
			IsError:    raw.Message.IsError,
		}
	}
	return e, true
}

// textOf joins the text blocks, one per line; images and tool blocks are skipped.
func textOf(blocks []rawBlock) string {
	var text strings.Builder
	for _, b := range blocks {
		if b.Type == "text" {
			text.WriteString(b.Text)
			text.WriteByte('\n')
		}
	}
	return text.String()
}

// parseBlocks accepts content as a block array or (tolerantly) a bare string.
func parseBlocks(content json.RawMessage) []rawBlock {
	if len(content) == 0 {
		return nil
	}
	var blocks []rawBlock
	if err := json.Unmarshal(content, &blocks); err == nil {
		return blocks
	}
	var s string
	if err := json.Unmarshal(content, &s); err == nil {
		return []rawBlock{{Type: "text", Text: s}}
	}
	return nil
}
