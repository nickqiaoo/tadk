package gemini

import (
	"testing"

	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/model"
	"google.golang.org/genai"
)

func TestThoughtSignatureEncodingRoundTrip(t *testing.T) {
	raw := []byte{0x12, 0xff, 0x03, 0x0a, 0x00, 0xfe}
	encoded := encodeThoughtSignatureBytes(raw)
	decoded, ok := decodeThoughtSignatureString(encoded)
	if !ok {
		t.Fatal("decodeThoughtSignatureString() = not ok, want ok")
	}
	if string(decoded) != string(raw) {
		t.Fatalf("decoded signature mismatch: got %v want %v", decoded, raw)
	}
}

func TestLegacyCorruptedThoughtSignatureIsIgnoredForGeminiReplay(t *testing.T) {
	a := &adapter{}
	msg := &message.Message{
		Role:     message.RoleAssistant,
		Protocol: string(model.ProtocolGeminiGenerateContent),
		Provider: string(model.ProviderGoogle),
		Model:    "gemini-3-flash-preview",
		Content: []message.Content{
			&message.ToolCallContent{
				ID:               "call-1",
				Name:             "bash",
				Arguments:        map[string]any{"command": "ls"},
				ThoughtSignature: "bad\ufffdsignature",
			},
		},
	}

	content := a.messageToGenai(msg, "gemini-3-flash-preview")
	if content == nil || len(content.Parts) != 1 || content.Parts[0].FunctionCall == nil {
		t.Fatalf("messageToGenai() did not return one function call part: %#v", content)
	}
	if got := string(content.Parts[0].ThoughtSignature); got != "skip_thought_signature_validator" {
		t.Fatalf("ThoughtSignature = %q, want placeholder", got)
	}
}

func TestGenaiContentToMessageEncodesThoughtSignatures(t *testing.T) {
	msg := genaiContentToMessage(&genai.Content{
		Role: "model",
		Parts: []*genai.Part{
			{
				FunctionCall: &genai.FunctionCall{
					ID:   "call-1",
					Name: "bash",
					Args: map[string]any{"command": "ls"},
				},
				ThoughtSignature: []byte{0x12, 0xff, 0x03},
			},
		},
	})

	toolCalls := msg.ToolCalls()
	if len(toolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(toolCalls))
	}
	if toolCalls[0].ThoughtSignature == "" {
		t.Fatal("ThoughtSignature was empty, want encoded value")
	}
	if _, ok := decodeThoughtSignatureString(toolCalls[0].ThoughtSignature); !ok {
		t.Fatal("encoded ThoughtSignature did not decode")
	}
}
