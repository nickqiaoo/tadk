package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/nickqiaoo/tadk/agent"
	"github.com/nickqiaoo/tadk/event"
	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/runner"
	"github.com/nickqiaoo/tadk/server/adkrest/internal/models"
	"github.com/nickqiaoo/tadk/session"
)

// RuntimeAPIController is the controller for the Runtime API.
type RuntimeAPIController struct {
	sseTimeout     time.Duration
	sessionService session.Service
	agentLoader    agent.Loader
}

// NewRuntimeAPIController creates the controller for the Runtime API.
func NewRuntimeAPIController(sessionService session.Service, agentLoader agent.Loader, sseTimeout time.Duration) *RuntimeAPIController {
	return &RuntimeAPIController{sessionService: sessionService, agentLoader: agentLoader, sseTimeout: sseTimeout}
}

// RunAgent executes a non-streaming agent run for a given session and message.
func (c *RuntimeAPIController) RunHandler(rw http.ResponseWriter, req *http.Request) error {
	runAgentRequest, err := decodeRequestBody(req)
	if err != nil {
		return err
	}
	sessionOutputs, err := c.runAgent(req.Context(), runAgentRequest)
	if err != nil {
		return err
	}
	EncodeJSONResponse(sessionOutputs, http.StatusOK, rw)
	return nil
}

// RunAgent executes a non-streaming agent run for a given session and message.
func (c *RuntimeAPIController) runAgent(ctx context.Context, runAgentRequest models.RunAgentRequest) ([]*message.Message, error) {
	err := c.validateSessionExists(ctx, runAgentRequest.AppName, runAgentRequest.UserId, runAgentRequest.SessionId)
	if err != nil {
		return nil, err
	}

	r, rCfg, err := c.getRunner(runAgentRequest)
	if err != nil {
		return nil, err
	}

	var msgs []*message.Message
	for ev, err := range r.Run(ctx, runAgentRequest.UserId, runAgentRequest.SessionId, &runAgentRequest.NewMessage, *rCfg) {
		if err != nil {
			return nil, newStatusError(fmt.Errorf("failed to run agent: %w", err), http.StatusInternalServerError)
		}
		if end, ok := ev.(*event.AgentEnd); ok {
			if end.Err != nil {
				return nil, newStatusError(fmt.Errorf("failed to run agent: %w", end.Err), http.StatusInternalServerError)
			}
			msgs = end.Messages
		}
	}
	return msgs, nil
}

// RunSSEHandler executes an agent run and streams the resulting entries using SSE.
func (c *RuntimeAPIController) RunSSEHandler(rw http.ResponseWriter, req *http.Request) error {
	rw.Header().Set("Content-Type", "text/event-stream")
	rw.Header().Set("Cache-Control", "no-cache")
	rw.Header().Set("Connection", "keep-alive")

	// set custom deadlines for this request - it overrides server-wide timeouts
	rc := http.NewResponseController(rw)
	deadline := time.Now().Add(c.sseTimeout)
	err := rc.SetWriteDeadline(deadline)
	if err != nil {
		return newStatusError(fmt.Errorf("failed to set write deadline: %w", err), http.StatusInternalServerError)
	}

	runAgentRequest, err := decodeRequestBody(req)
	if err != nil {
		return err
	}

	err = c.validateSessionExists(req.Context(), runAgentRequest.AppName, runAgentRequest.UserId, runAgentRequest.SessionId)
	if err != nil {
		return err
	}

	r, rCfg, err := c.getRunner(runAgentRequest)
	if err != nil {
		return err
	}

	events := r.Run(req.Context(), runAgentRequest.UserId, runAgentRequest.SessionId, &runAgentRequest.NewMessage, *rCfg)

	for output, err := range events {
		if err != nil {
			_, err := fmt.Fprintf(rw, "Error while running agent: %v\n", err)
			if err != nil {
				return newStatusError(fmt.Errorf("failed to write response: %w", err), http.StatusInternalServerError)
			}
			err = rc.Flush()
			if err != nil {
				return newStatusError(fmt.Errorf("failed to flush: %w", err), http.StatusInternalServerError)
			}

			continue
		}
		if err := runner.WriteRawAgentEventSSE(rw, output, runner.RawAgentEventSSEOptions{}); err != nil {
			return newStatusError(err, http.StatusInternalServerError)
		}
		if err := rc.Flush(); err != nil {
			return newStatusError(fmt.Errorf("failed to flush: %w", err), http.StatusInternalServerError)
		}
	}
	return nil
}

func (c *RuntimeAPIController) validateSessionExists(ctx context.Context, appName, userID, sessionID string) error {
	_, err := c.sessionService.Get(ctx, &session.GetRequest{
		AppName:   appName,
		UserID:    userID,
		SessionID: sessionID,
	})
	if err != nil {
		return newStatusError(fmt.Errorf("failed to get session: %w", err), http.StatusNotFound)
	}
	return nil
}

func (c *RuntimeAPIController) getRunner(req models.RunAgentRequest) (*runner.Runner, *agent.RunOptions, error) {
	curAgent, err := c.agentLoader.LoadAgent(req.AppName)
	if err != nil {
		return nil, nil, newStatusError(fmt.Errorf("failed to load agent: %w", err), http.StatusInternalServerError)
	}

	r, err := runner.New(runner.Config{
		AppName:        req.AppName,
		Agent:          curAgent,
		SessionService: c.sessionService,
	},
	)
	if err != nil {
		return nil, nil, newStatusError(fmt.Errorf("failed to create runner: %w", err), http.StatusInternalServerError)
	}

	streamingMode := agent.StreamingModeNone
	if req.Streaming {
		streamingMode = agent.StreamingModeSSE
	}
	return r, &agent.RunOptions{
		StreamingMode: streamingMode,
	}, nil
}

func decodeRequestBody(req *http.Request) (decodedReq models.RunAgentRequest, err error) {
	var runAgentRequest models.RunAgentRequest
	defer func() {
		err = req.Body.Close()
	}()
	d := json.NewDecoder(req.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(&runAgentRequest); err != nil {
		return runAgentRequest, newStatusError(fmt.Errorf("failed to decode request: %w", err), http.StatusBadRequest)
	}
	return runAgentRequest, nil
}
