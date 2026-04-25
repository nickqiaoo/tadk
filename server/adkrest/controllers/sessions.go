package controllers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/nickqiaoo/tadk/server/adkrest/internal/models"
	"github.com/nickqiaoo/tadk/session"
)

// TODO: Confirm error handling and target semantic for REST API.

// SessionsAPIController is the controller for the Sessions API.
type SessionsAPIController struct {
	service session.Service
}

// NewSessionsAPIController creates a new SessionsAPIController.
func NewSessionsAPIController(service session.Service) *SessionsAPIController {
	return &SessionsAPIController{service: service}
}

// CreateSesssionHTTP is a HTTP handler for the create session API.
func (c *SessionsAPIController) CreateSessionHandler(rw http.ResponseWriter, req *http.Request) {
	params := mux.Vars(req)
	sessionID, err := models.SessionIDFromHTTPParameters(params)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}
	createSessionRequest := models.CreateSessionRequest{}
	// No state and no entries, fails to decode req.Body failing with "EOF"
	if req.ContentLength > 0 {
		err := json.NewDecoder(req.Body).Decode(&createSessionRequest)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusBadRequest)
			return
		}
	}
	respSession, err := c.createSession(req.Context(), sessionID, createSessionRequest)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	EncodeJSONResponse(respSession, http.StatusOK, rw)
}

func (c *SessionsAPIController) createSession(ctx context.Context, sessionID models.SessionID, createSessionRequest models.CreateSessionRequest) (models.Session, error) {
	createResp, err := c.service.Create(ctx, &session.CreateRequest{
		AppName:   sessionID.AppName,
		UserID:    sessionID.UserID,
		SessionID: sessionID.ID,
	})
	if err != nil {
		return models.Session{}, err
	}
	for _, requestEntry := range createSessionRequest.Entries {
		entry := models.ToSessionEntry(requestEntry, lastEntryID(createResp.Session))
		if entry == nil {
			continue
		}
		err = c.service.AppendEntry(ctx, &session.AppendEntryRequest{
			AppName:   sessionID.AppName,
			UserID:    sessionID.UserID,
			SessionID: sessionID.ID,
			Entry:     entry,
		})
		if err != nil {
			return models.Session{}, err
		}
	}
	return models.FromSession(createResp.Session)
}

func lastEntryID(sess session.Session) string {
	if sess == nil || sess.Entries().Len() == 0 {
		return ""
	}
	if b := sess.Entries().At(sess.Entries().Len() - 1).Base(); b != nil {
		return b.ID
	}
	return ""
}

// DeleteSession handles deleting a specific session.
func (c *SessionsAPIController) DeleteSessionHandler(rw http.ResponseWriter, req *http.Request) {
	params := mux.Vars(req)
	sessionID, err := models.SessionIDFromHTTPParameters(params)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}
	if sessionID.ID == "" {
		http.Error(rw, "session_id parameter is required", http.StatusBadRequest)
		return
	}

	err = c.service.Delete(req.Context(), &session.DeleteRequest{
		AppName:   sessionID.AppName,
		UserID:    sessionID.UserID,
		SessionID: sessionID.ID,
	})
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	EncodeJSONResponse(nil, http.StatusOK, rw)
}

// GetSession retrieves a specific session by its ID.
func (c *SessionsAPIController) GetSessionHandler(rw http.ResponseWriter, req *http.Request) {
	params := mux.Vars(req)
	sessionID, err := models.SessionIDFromHTTPParameters(params)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}
	if sessionID.ID == "" {
		http.Error(rw, "session_id parameter is required", http.StatusBadRequest)
		return
	}
	storedSession, err := c.service.Get(req.Context(), &session.GetRequest{
		AppName:   sessionID.AppName,
		UserID:    sessionID.UserID,
		SessionID: sessionID.ID,
	})
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	session, err := models.FromSession(storedSession.Session)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	EncodeJSONResponse(session, http.StatusOK, rw)
}

// ListSessions handles listing all sessions for a given app and user.
func (c *SessionsAPIController) ListSessionsHandler(rw http.ResponseWriter, req *http.Request) {
	params := mux.Vars(req)
	sessionID, err := models.SessionIDFromHTTPParameters(params)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}
	var sessions []models.Session
	resp, err := c.service.List(req.Context(), &session.ListRequest{
		AppName: sessionID.AppName,
		UserID:  sessionID.UserID,
	})
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, session := range resp.Sessions {
		respSession, err := models.FromSession(session)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}
		sessions = append(sessions, respSession)
	}
	EncodeJSONResponse(sessions, http.StatusOK, rw)
}
