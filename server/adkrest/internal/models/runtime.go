package models

import (
	"fmt"

	"github.com/nickqiaoo/tadk/message"
)

type RunAgentRequest struct {
	AppName string `json:"appName"`

	UserId string `json:"userId"`

	SessionId string `json:"sessionId"`

	NewMessage message.Message `json:"newMessage"`

	Streaming bool `json:"streaming,omitempty"`
}

// AssertRunAgentRequestRequired checks if the required fields are not zero-ed
func (req RunAgentRequest) AssertRunAgentRequestRequired() error {
	elements := map[string]any{
		"appName":    req.AppName,
		"userId":     req.UserId,
		"sessionId":  req.SessionId,
		"newMessage": req.NewMessage,
	}
	for name, el := range elements {
		if isZero := IsZeroValue(el); isZero {
			return fmt.Errorf("%s is required", name)
		}
	}

	return nil
}
