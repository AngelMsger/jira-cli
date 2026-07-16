package apiclient

import (
	"encoding/json"
	"strings"
)

// adf.go bridges the body-representation asymmetry between the flavors: Jira
// Cloud (REST v3) expects and returns Atlassian Document Format (ADF) JSON for
// rich-text fields (description, comment bodies), while Data Center (REST v2)
// uses plain strings (optionally Jira wiki markup, rendered server-side).
//
// The CLI's contract is plain text on both flavors: writes convert text to a
// minimal ADF document on Cloud, reads flatten ADF back to text. The
// conversion is intentionally lossy for rich nodes (tables, panels); see the
// capability table (capability.go) and the companion Skill for the caveats.

// TextToADF builds a minimal ADF document from plain text: paragraphs are
// separated by blank lines, single newlines inside a paragraph become
// hardBreak nodes. Empty input yields a document with no content, which Jira
// accepts as an empty body.
func TextToADF(text string) map[string]any {
	content := []any{}
	for _, para := range splitParagraphs(text) {
		nodes := []any{}
		for i, line := range strings.Split(para, "\n") {
			if i > 0 {
				nodes = append(nodes, map[string]any{"type": "hardBreak"})
			}
			if line != "" {
				nodes = append(nodes, map[string]any{"type": "text", "text": line})
			}
		}
		if len(nodes) == 0 {
			continue
		}
		content = append(content, map[string]any{"type": "paragraph", "content": nodes})
	}
	return map[string]any{"type": "doc", "version": 1, "content": content}
}

// splitParagraphs splits text on runs of blank lines. Windows line endings are
// normalized first.
func splitParagraphs(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	var paras []string
	for _, chunk := range strings.Split(text, "\n\n") {
		chunk = strings.Trim(chunk, "\n")
		if chunk != "" {
			paras = append(paras, chunk)
		}
	}
	return paras
}

// adfNode is the generic shape of an ADF node, for reading.
type adfNode struct {
	Type    string         `json:"type"`
	Text    string         `json:"text"`
	Attrs   map[string]any `json:"attrs"`
	Content []adfNode      `json:"content"`
}

// ADFToText flattens an ADF document (as raw JSON) into plain text. Block
// nodes become paragraphs separated by blank lines; hardBreaks become
// newlines; mentions and emoji render their display text; unknown nodes are
// recursed into so no text content is silently dropped.
func ADFToText(raw json.RawMessage) string {
	var doc adfNode
	if json.Unmarshal(raw, &doc) != nil {
		return ""
	}
	var b strings.Builder
	writeADFBlocks(&b, doc.Content)
	return strings.TrimRight(b.String(), "\n")
}

// writeADFBlocks renders a sequence of block-level nodes, separating them with
// blank lines.
func writeADFBlocks(b *strings.Builder, nodes []adfNode) {
	for _, n := range nodes {
		start := b.Len()
		writeADFBlock(b, n, "")
		if b.Len() > start {
			b.WriteString("\n\n")
		}
	}
}

// writeADFBlock renders one block node. prefix is prepended to list items.
func writeADFBlock(b *strings.Builder, n adfNode, prefix string) {
	switch n.Type {
	case "bulletList", "orderedList":
		first := true
		for _, item := range n.Content {
			if !first {
				b.WriteString("\n")
			}
			first = false
			b.WriteString(prefix + "- ")
			writeADFInline(b, item.Content)
		}
	case "codeBlock":
		writeADFInline(b, n.Content)
	case "blockquote":
		for i, child := range n.Content {
			if i > 0 {
				b.WriteString("\n")
			}
			b.WriteString("> ")
			writeADFInline(b, child.Content)
		}
	default:
		// paragraph, heading, table rows, panels, unknown containers: flatten.
		if len(n.Content) > 0 {
			writeADFInline(b, n.Content)
		} else if n.Text != "" {
			b.WriteString(n.Text)
		}
	}
}

// writeADFInline renders inline content: text runs, breaks, mentions, and any
// nested containers (paragraphs inside list items, etc).
func writeADFInline(b *strings.Builder, nodes []adfNode) {
	for _, n := range nodes {
		switch n.Type {
		case "text":
			b.WriteString(n.Text)
		case "hardBreak":
			b.WriteString("\n")
		case "mention", "emoji", "status":
			if t, ok := n.Attrs["text"].(string); ok && t != "" {
				b.WriteString(t)
			}
		case "inlineCard":
			if u, ok := n.Attrs["url"].(string); ok && u != "" {
				b.WriteString(u)
			}
		case "paragraph":
			writeADFInline(b, n.Content)
		default:
			if n.Text != "" {
				b.WriteString(n.Text)
			}
			writeADFInline(b, n.Content)
		}
	}
}

// bodyToText normalizes a rich-text field from a raw API value: Cloud returns
// an ADF object, Data Center returns a JSON string (or null).
func bodyToText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	if raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return s
		}
		return ""
	}
	return ADFToText(raw)
}

// textToBody converts plain text into the flavor's rich-text write shape: an
// ADF document on Cloud, the string itself on Data Center.
func (c *apiClient) textToBody(text string) any {
	if c.flavor == FlavorCloud {
		return TextToADF(text)
	}
	return text
}
