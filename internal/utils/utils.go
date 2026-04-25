package utils

import (
	"strings"

	"github.com/google/uuid"
	"google.golang.org/genai"

	"github.com/nickqiaoo/tadk/agent"
	"github.com/nickqiaoo/tadk/model"
)

// TODO: split in proper files/packages.

const afFunctionCallIDPrefix = "adk-"

// PopulateClientFunctionCallID sets the function call ID field if it is empty.
// Since the ID field is optional, some models don't fill the field, but
// the LLMAgent depends on the IDs to map FunctionCall and FunctionResponse events
// in the event stream.
func PopulateClientFunctionCallID(c *genai.Content) {
	for _, fn := range FunctionCalls(c) {
		if fn.ID == "" {
			fn.ID = GenerateFunctionCallID()
		}
	}
}

// GenerateFunctionCallID generates a new function call ID.
func GenerateFunctionCallID() string {
	return afFunctionCallIDPrefix + uuid.NewString()
}

// RemoveClientFunctionCallID removes the function call ID field that was set
// by populateClientFunctionCallID. This is necessary when FunctionCall or
// FunctionResponse are sent back to the model.
func RemoveClientFunctionCallID(c *genai.Content) {
	for _, fn := range FunctionCalls(c) {
		if strings.HasPrefix(fn.ID, afFunctionCallIDPrefix) {
			fn.ID = ""
		}
	}
	for _, fn := range FunctionResponses(c) {
		if strings.HasPrefix(fn.ID, afFunctionCallIDPrefix) {
			fn.ID = ""
		}
	}
}

// Belows are useful utilities that help working with genai.Content
// included in types.Event.
// TODO: Use generics.
// FunctionCalls extracts all FunctionCall parts from the content.
func FunctionCalls(c *genai.Content) (ret []*genai.FunctionCall) {
	if c == nil {
		return nil
	}
	for _, p := range c.Parts {
		if p.FunctionCall != nil {
			ret = append(ret, p.FunctionCall)
		}
	}
	return ret
}

// FunctionResponses extracts all FunctionResponse parts from the content.
func FunctionResponses(c *genai.Content) (ret []*genai.FunctionResponse) {
	if c == nil {
		return nil
	}
	for _, p := range c.Parts {
		if p.FunctionResponse != nil {
			ret = append(ret, p.FunctionResponse)
		}
	}
	return ret
}

// TextParts extracts all Text parts from the content.
func TextParts(c *genai.Content) (ret []string) {
	if c == nil {
		return nil
	}
	for _, p := range c.Parts {
		if p.Text != "" {
			ret = append(ret, p.Text)
		}
	}
	return ret
}

// FunctionDecls extracts all Function declarations from the GenerateContentConfig.
func FunctionDecls(c *genai.GenerateContentConfig) (ret []*genai.FunctionDeclaration) {
	if c == nil {
		return nil
	}
	for _, t := range c.Tools {
		ret = append(ret, t.FunctionDeclarations...)
	}
	return ret
}

func Must[T agent.Agent](a T, err error) T {
	if err != nil {
		panic(err)
	}
	return a
}

func AppendInstructions(r *model.Request, instructions ...string) {
	if len(instructions) == 0 {
		return
	}
	inst := strings.Join(instructions, "\n\n")
	if r.SystemPrompt == "" {
		r.SystemPrompt = inst
	} else {
		r.SystemPrompt = r.SystemPrompt + "\n\n" + inst
	}
}
