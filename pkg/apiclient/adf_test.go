package apiclient

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestTextToADF(t *testing.T) {
	t.Parallel()
	got := TextToADF("first line\nsecond line\n\nnew paragraph")
	want := map[string]any{
		"type": "doc", "version": 1,
		"content": []any{
			map[string]any{"type": "paragraph", "content": []any{
				map[string]any{"type": "text", "text": "first line"},
				map[string]any{"type": "hardBreak"},
				map[string]any{"type": "text", "text": "second line"},
			}},
			map[string]any{"type": "paragraph", "content": []any{
				map[string]any{"type": "text", "text": "new paragraph"},
			}},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("TextToADF mismatch\n got:  %#v\n want: %#v", got, want)
	}
}

func TestTextToADFEmpty(t *testing.T) {
	t.Parallel()
	got := TextToADF("")
	content, ok := got["content"].([]any)
	if !ok || len(content) != 0 {
		t.Errorf("TextToADF(\"\") content = %#v, want empty", got["content"])
	}
}

func TestTextToADFWindowsLineEndings(t *testing.T) {
	t.Parallel()
	got := TextToADF("a\r\n\r\nb")
	content := got["content"].([]any)
	if len(content) != 2 {
		t.Fatalf("expected 2 paragraphs, got %d: %#v", len(content), content)
	}
}

func TestADFToText(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		adf  string
		want string
	}{
		{"paragraphs and breaks",
			`{"type":"doc","version":1,"content":[
				{"type":"paragraph","content":[
					{"type":"text","text":"first"},
					{"type":"hardBreak"},
					{"type":"text","text":"second"}]},
				{"type":"paragraph","content":[{"type":"text","text":"third"}]}]}`,
			"first\nsecond\n\nthird"},
		{"mention",
			`{"type":"doc","version":1,"content":[
				{"type":"paragraph","content":[
					{"type":"text","text":"ping "},
					{"type":"mention","attrs":{"id":"123","text":"@Alice"}}]}]}`,
			"ping @Alice"},
		{"bullet list",
			`{"type":"doc","version":1,"content":[
				{"type":"bulletList","content":[
					{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"one"}]}]},
					{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"two"}]}]}]}]}`,
			"- one\n- two"},
		{"code block",
			`{"type":"doc","version":1,"content":[
				{"type":"codeBlock","attrs":{"language":"go"},"content":[{"type":"text","text":"x := 1"}]}]}`,
			"x := 1"},
		{"unknown node recursed",
			`{"type":"doc","version":1,"content":[
				{"type":"panel","attrs":{"panelType":"info"},"content":[
					{"type":"paragraph","content":[{"type":"text","text":"note"}]}]}]}`,
			"note"},
		{"empty doc", `{"type":"doc","version":1,"content":[]}`, ""},
		{"invalid json", `{not json`, ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ADFToText(json.RawMessage(tc.adf))
			if got != tc.want {
				t.Errorf("ADFToText\n got:  %q\n want: %q", got, tc.want)
			}
		})
	}
}

func TestBodyToText(t *testing.T) {
	t.Parallel()
	if got := bodyToText(json.RawMessage(`"plain DC string"`)); got != "plain DC string" {
		t.Errorf("string body = %q", got)
	}
	if got := bodyToText(json.RawMessage(`null`)); got != "" {
		t.Errorf("null body = %q, want empty", got)
	}
	if got := bodyToText(nil); got != "" {
		t.Errorf("nil body = %q, want empty", got)
	}
	adf := `{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"hi"}]}]}`
	if got := bodyToText(json.RawMessage(adf)); got != "hi" {
		t.Errorf("adf body = %q, want hi", got)
	}
}

// TestTextToADFRoundTrip pins the write→read contract: text written through
// TextToADF reads back identically through ADFToText.
func TestTextToADFRoundTrip(t *testing.T) {
	t.Parallel()
	for _, text := range []string{
		"single",
		"line one\nline two",
		"para one\n\npara two\nwith break",
	} {
		doc, err := json.Marshal(TextToADF(text))
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if got := ADFToText(doc); got != text {
			t.Errorf("round trip %q -> %q", text, got)
		}
	}
}
