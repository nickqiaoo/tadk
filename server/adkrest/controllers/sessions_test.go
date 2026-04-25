package controllers_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/gorilla/mux"

	"github.com/nickqiaoo/tadk/server/adkrest/controllers"
	"github.com/nickqiaoo/tadk/server/adkrest/internal/fakes"
	"github.com/nickqiaoo/tadk/server/adkrest/internal/models"
)

func TestGetSession(t *testing.T) {
	id := fakes.SessionKey{
		AppName:   "testApp",
		UserID:    "testUser",
		SessionID: "testSession",
	}

	tc := []struct {
		name           string
		storedSessions map[fakes.SessionKey]*fakes.TestSession
		sessionID      fakes.SessionKey
		wantSession    models.Session
		wantErr        error
		wantStatus     int
	}{
		{
			name: "session exists",
			storedSessions: map[fakes.SessionKey]*fakes.TestSession{
				id: {
					Id:             id,
					SessionEntries: fakes.TestEntries{},
					UpdatedAt:      time.Now(),
				},
			},
			sessionID: id,
			wantSession: models.Session{
				ID:        "testSession",
				AppName:   "testApp",
				UserID:    "testUser",
				UpdatedAt: time.Now().Unix(),
				Entries:   []models.Entry{},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:           "session does not exist",
			storedSessions: map[fakes.SessionKey]*fakes.TestSession{},
			sessionID:      id,
			wantErr:        fmt.Errorf("not found"),
			wantStatus:     http.StatusInternalServerError,
		},
		{
			name: "user ID is missing in input",
			storedSessions: map[fakes.SessionKey]*fakes.TestSession{
				id: {
					Id:             id,
					SessionEntries: fakes.TestEntries{},
					UpdatedAt:      time.Now(),
				},
			},
			sessionID: fakes.SessionKey{
				AppName:   "testApp",
				SessionID: "testSession",
			},
			wantErr:    fmt.Errorf("user_id parameter is required"),
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "session ID is missing",
			storedSessions: map[fakes.SessionKey]*fakes.TestSession{
				id: {
					Id: fakes.SessionKey{
						AppName: "testApp",
						UserID:  "testUser",
					},
					SessionEntries: fakes.TestEntries{},
					UpdatedAt:      time.Now(),
				},
			},
			sessionID:  id,
			wantErr:    fmt.Errorf("session_id is empty in received session"),
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			sessionService := fakes.FakeSessionService{Sessions: tt.storedSessions}
			apiController := controllers.NewSessionsAPIController(&sessionService)
			req, err := http.NewRequest(http.MethodGet, "/apps/testApp/users/testUser/sessions/testSession", nil)
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			// Manually set the URL variables on the request using mux.SetURLVars.
			req = mux.SetURLVars(req, sessionVars(tt.sessionID))
			rr := httptest.NewRecorder()

			apiController.GetSessionHandler(rr, req)

			if status := rr.Code; status != tt.wantStatus {
				t.Fatalf("handler returned wrong status code: got %v want %v", status, tt.wantStatus)
			}
			if tt.wantErr != nil {
				respErr := strings.Trim(rr.Body.String(), "\n")
				if tt.wantErr.Error() != respErr {
					t.Errorf("CreateSession() mismatch (-want +got):\n%v, %v", tt.wantErr.Error(), respErr)
				}
				return
			}
			var gotSession models.Session
			err = json.NewDecoder(rr.Body).Decode(&gotSession)
			if err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if diff := cmp.Diff(tt.wantSession, gotSession, EquateApproxInt(int64(time.Second))); diff != "" {
				t.Errorf("GetSession() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestCreateSession(t *testing.T) {
	id := fakes.SessionKey{
		AppName:   "testApp",
		UserID:    "testUser",
		SessionID: "testSession",
	}

	tc := []struct {
		name             string
		storedSessions   map[fakes.SessionKey]*fakes.TestSession
		sessionID        fakes.SessionKey
		createRequestObj models.CreateSessionRequest
		wantSession      models.Session
		wantErr          error
		wantStatus       int
	}{
		{
			name: "session exists",
			storedSessions: map[fakes.SessionKey]*fakes.TestSession{
				id: {
					Id:             id,
					SessionEntries: fakes.TestEntries{},
					UpdatedAt:      time.Now(),
				},
			},
			sessionID:  id,
			wantErr:    fmt.Errorf("session already exists"),
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:           "successful create operation",
			storedSessions: map[fakes.SessionKey]*fakes.TestSession{},
			sessionID:      id,
			createRequestObj: models.CreateSessionRequest{
				Entries: []models.Entry{
					{
						ID:     "entryID",
						Time:   time.Now().Add(5 * time.Minute).Unix(),
						Author: "testUser",
					},
				},
			},
			wantSession: models.Session{
				ID:        "testSession",
				AppName:   "testApp",
				UserID:    "testUser",
				UpdatedAt: time.Now().Add(5 * time.Minute).Unix(),
				Entries: []models.Entry{
					{
						ID:     "entryID",
						Author: "testUser",
						Time:   time.Now().Add(5 * time.Minute).Unix(),
					},
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:           "user id is missing",
			storedSessions: map[fakes.SessionKey]*fakes.TestSession{},
			sessionID: fakes.SessionKey{
				AppName:   "testApp",
				SessionID: "testSession",
			},
			createRequestObj: models.CreateSessionRequest{},
			wantStatus:       http.StatusBadRequest,
			wantErr:          fmt.Errorf("user_id parameter is required"),
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			sessionService := fakes.FakeSessionService{Sessions: tt.storedSessions}
			apiController := controllers.NewSessionsAPIController(&sessionService)
			reqBytes, err := json.Marshal(tt.createRequestObj)
			if err != nil {
				t.Fatalf("marshal request: %v", err)
			}
			req, err := http.NewRequest(http.MethodPost, "/apps/testApp/users/testUser/sessions/testSession", bytes.NewBuffer(reqBytes))
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			// Manually set the URL variables on the request using mux.SetURLVars.
			req = mux.SetURLVars(req, sessionVars(tt.sessionID))
			rr := httptest.NewRecorder()

			apiController.CreateSessionHandler(rr, req)

			if status := rr.Code; status != tt.wantStatus {
				t.Errorf("handler returned wrong status code: got %v want %v", status, tt.wantStatus)
			}
			if tt.wantErr != nil {
				respErr := strings.Trim(rr.Body.String(), "\n")
				if tt.wantErr.Error() != respErr {
					t.Errorf("CreateSession() mismatch (-want +got):\n%v, %v", tt.wantErr.Error(), respErr)
				}
				return
			}
			var gotSession models.Session
			err = json.NewDecoder(rr.Body).Decode(&gotSession)
			if err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if diff := cmp.Diff(tt.wantSession, gotSession, EquateApproxInt(int64(time.Second))); diff != "" {
				t.Errorf("CreateSession() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDeleteSession(t *testing.T) {
	id := fakes.SessionKey{
		AppName:   "testApp",
		UserID:    "testUser",
		SessionID: "testSession",
	}

	tc := []struct {
		name           string
		storedSessions map[fakes.SessionKey]*fakes.TestSession
		sessionID      fakes.SessionKey
		wantStatus     int
	}{
		{
			name: "session exists",
			storedSessions: map[fakes.SessionKey]*fakes.TestSession{
				id: {
					Id:             id,
					SessionEntries: fakes.TestEntries{},
					UpdatedAt:      time.Now(),
				},
			},
			sessionID:  id,
			wantStatus: http.StatusOK,
		},
		{
			name:           "session does not exist",
			storedSessions: map[fakes.SessionKey]*fakes.TestSession{},
			sessionID:      id,
			wantStatus:     http.StatusInternalServerError,
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			sessionService := fakes.FakeSessionService{Sessions: tt.storedSessions}
			apiController := controllers.NewSessionsAPIController(&sessionService)
			req, err := http.NewRequest(http.MethodDelete, "/apps/testApp/users/testUser/sessions/testSession", nil)
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			// Manually set the URL variables on the request using mux.SetURLVars.
			req = mux.SetURLVars(req, sessionVars(tt.sessionID))
			rr := httptest.NewRecorder()

			apiController.DeleteSessionHandler(rr, req)
			if status := rr.Code; status != tt.wantStatus {
				t.Fatalf("handler returned wrong status code: got %v want %v", status, tt.wantStatus)
			}
			if _, ok := sessionService.Sessions[tt.sessionID]; ok {
				t.Errorf("session was not deleted")
			}
		})
	}
}

func TestListSessions(t *testing.T) {
	id := fakes.SessionKey{
		AppName:   "testApp",
		UserID:    "testUser",
		SessionID: "testSession",
	}
	newSessionID := fakes.SessionKey{
		AppName:   "testApp",
		UserID:    "testUser",
		SessionID: "newSession",
	}
	oldSessionID := fakes.SessionKey{
		AppName:   "testApp",
		UserID:    "testUser",
		SessionID: "oldSession",
	}

	tc := []struct {
		name           string
		storedSessions map[fakes.SessionKey]*fakes.TestSession
		wantSessions   []models.Session
		wantStatus     int
	}{
		{
			name: "session exists",
			storedSessions: map[fakes.SessionKey]*fakes.TestSession{
				id: {
					Id:             id,
					SessionEntries: fakes.TestEntries{},
					UpdatedAt:      time.Now(),
				},
				newSessionID: {
					Id:             newSessionID,
					SessionEntries: fakes.TestEntries{},
					UpdatedAt:      time.Now(),
				},
				oldSessionID: {
					Id:             oldSessionID,
					SessionEntries: fakes.TestEntries{},
					UpdatedAt:      time.Now(),
				},
			},
			wantSessions: []models.Session{
				{
					ID:        "testSession",
					AppName:   "testApp",
					UserID:    "testUser",
					UpdatedAt: time.Now().Unix(),
					Entries:   []models.Entry{},
				},
				{
					ID:        "newSession",
					AppName:   "testApp",
					UserID:    "testUser",
					UpdatedAt: time.Now().Unix(),
					Entries:   []models.Entry{},
				},
				{
					ID:        "oldSession",
					AppName:   "testApp",
					UserID:    "testUser",
					UpdatedAt: time.Now().Unix(),
					Entries:   []models.Entry{},
				},
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			sessionService := fakes.FakeSessionService{Sessions: tt.storedSessions}
			apiController := controllers.NewSessionsAPIController(&sessionService)
			req, err := http.NewRequest(http.MethodDelete, "/apps/testApp/users/testUser/sessions/testSession", nil)
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			// Manually set the URL variables on the request using mux.SetURLVars.
			req = mux.SetURLVars(req, map[string]string{
				"app_name": "testApp",
				"user_id":  "testUser",
			})
			rr := httptest.NewRecorder()

			apiController.ListSessionsHandler(rr, req)
			if status := rr.Code; status != tt.wantStatus {
				t.Fatalf("handler returned wrong status code: got %v want %v", status, tt.wantStatus)
			}
			got := []models.Session{}
			err = json.NewDecoder(rr.Body).Decode(&got)
			if err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if diff := cmp.Diff(tt.wantSessions, got, EquateApproxInt(int64(time.Second)), cmpopts.SortSlices(func(a, b models.Session) bool {
				return a.ID < b.ID
			})); diff != "" {
				t.Errorf("ListSessions() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func sessionVars(sessionID fakes.SessionKey) map[string]string {
	return map[string]string{
		"app_name":   sessionID.AppName,
		"user_id":    sessionID.UserID,
		"session_id": sessionID.SessionID,
	}
}

// EquateApproxInt returns a cmp.Comparer option that determines integer values
// to be equal if they are within a certain absolute margin.
func EquateApproxInt(margin int64) cmp.Option {
	return cmp.Comparer(func(x, y int64) bool {
		diff := x - y
		if diff < 0 {
			diff = -diff
		}

		return diff <= margin
	})
}
