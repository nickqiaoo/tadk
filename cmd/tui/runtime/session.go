package runtime

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/session"
	filesession "github.com/nickqiaoo/tadk/session/file"
	interrupttool "github.com/nickqiaoo/tadk/tool/interrupt"
)

type SessionSummary struct {
	ID               string
	UpdatedAt        time.Time
	PendingInterrupt int
	Preview          string
}

type SessionManager struct {
	AppName string
	UserID  string
	Service session.Service
}

func NewSessionManager(appName, userID, dir string) (*SessionManager, error) {
	svc, err := filesession.NewService(filesession.ServiceConfig{Dir: dir})
	if err != nil {
		return nil, err
	}
	return &SessionManager{
		AppName: appName,
		UserID:  userID,
		Service: svc,
	}, nil
}

func (s *SessionManager) New(ctx context.Context, sessionID string) (session.Session, error) {
	resp, err := s.Service.Create(ctx, &session.CreateRequest{
		AppName:   s.AppName,
		UserID:    s.UserID,
		SessionID: strings.TrimSpace(sessionID),
	})
	if err != nil {
		return nil, err
	}
	return resp.Session, nil
}

func (s *SessionManager) Load(ctx context.Context, sessionID string) (session.Session, error) {
	resp, err := s.Service.Get(ctx, &session.GetRequest{
		AppName:   s.AppName,
		UserID:    s.UserID,
		SessionID: strings.TrimSpace(sessionID),
	})
	if err != nil {
		return nil, err
	}
	return resp.Session, nil
}

func (s *SessionManager) List(ctx context.Context) ([]SessionSummary, error) {
	resp, err := s.Service.List(ctx, &session.ListRequest{
		AppName: s.AppName,
		UserID:  s.UserID,
	})
	if err != nil {
		return nil, err
	}

	items := make([]SessionSummary, 0, len(resp.Sessions))
	for _, sess := range resp.Sessions {
		if sess == nil {
			continue
		}
		summary := SessionSummary{
			ID:        sess.ID(),
			UpdatedAt: sess.LastUpdateTime(),
		}
		cp, err := s.Checkpoint(ctx, sess.ID())
		if err == nil && cp != nil {
			summary.PendingInterrupt = len(cp.PendingInterrupts)
		}
		// Load full session to extract first user message preview
		if loaded, err := s.Load(ctx, sess.ID()); err == nil && loaded != nil {
			for entry := range loaded.Entries().All() {
				if msgEntry, ok := entry.(*session.MessageEntry); ok && msgEntry.Message != nil {
					if msgEntry.Message.Role == message.RoleUser {
						text := strings.TrimSpace(msgEntry.Message.Text())
						if text != "" {
							summary.Preview = previewText(text, 60)
							break
						}
					}
				}
			}
		}
		items = append(items, summary)
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	return items, nil
}

func (s *SessionManager) Checkpoint(ctx context.Context, sessionID string) (*session.Checkpoint, error) {
	return s.Service.GetCheckpoint(ctx, &session.CheckpointRequest{
		AppName:   s.AppName,
		UserID:    s.UserID,
		SessionID: strings.TrimSpace(sessionID),
	})
}

func (s *SessionManager) EnsureCheckpointApprovalDetails(ctx context.Context, sessionID string) (*session.Checkpoint, error) {
	cp, err := s.Checkpoint(ctx, sessionID)
	if err != nil || cp == nil {
		return cp, err
	}

	changed := false
	for _, interruptID := range SortedPendingInterruptIDs(cp) {
		pending := cp.PendingInterrupts[interruptID]
		if pending == nil || pending.ToolCallID == "" {
			continue
		}
		info, ok := approvalInfoFromPending(pending.Info)
		if !ok {
			continue
		}
		if pending.ToolName == "" && info.Tool != "" {
			pending.ToolName = info.Tool
			changed = true
		}
		if len(pending.ToolArgs) == 0 && len(info.Args) > 0 {
			pending.ToolArgs = cloneAnyMap(info.Args)
			changed = true
		}
	}

	if !changed {
		return cp, nil
	}

	if err := s.Service.SaveCheckpoint(ctx, &session.SaveCheckpointRequest{
		CheckpointRequest: session.CheckpointRequest{
			AppName:   s.AppName,
			UserID:    s.UserID,
			SessionID: strings.TrimSpace(sessionID),
		},
		Checkpoint:       cp,
		ExpectedRevision: cp.Revision,
	}); err != nil {
		return nil, err
	}

	return s.Checkpoint(ctx, sessionID)
}

func (s *SessionManager) RecordModelChange(ctx context.Context, sessionID string, choice ModelChoice, entries session.Entries) error {
	choice = ModelChoice{
		Provider: strings.TrimSpace(strings.ToLower(choice.Provider)),
		Name:     strings.TrimSpace(choice.Name),
	}
	if !choice.Valid() {
		return nil
	}
	lastParentID := ""
	if entries != nil && entries.Len() > 0 {
		if last := entries.At(entries.Len() - 1); last != nil && last.Base() != nil {
			lastParentID = last.Base().ID
		}
	}

	entry := &session.ModelChangeEntry{
		EntryBase: session.NewEntryBase(session.EntryTypeModelChange, lastParentID),
		Provider:  choice.Provider,
		ModelID:   choice.Name,
	}
	return s.Service.AppendEntry(ctx, &session.AppendEntryRequest{
		AppName:   s.AppName,
		UserID:    s.UserID,
		SessionID: sessionID,
		Entry:     entry,
	})
}

func (s *SessionManager) Close() error {
	type closer interface {
		Close() error
	}
	c, ok := s.Service.(closer)
	if !ok {
		return nil
	}
	return c.Close()
}

func CollectEntries(entries session.Entries) []session.DurableEntry {
	if entries == nil {
		return nil
	}
	out := make([]session.DurableEntry, 0, entries.Len())
	for entry := range entries.All() {
		out = append(out, entry)
	}
	return out
}

func SessionModelChoice(sess session.Session) (ModelChoice, bool) {
	if sess == nil {
		return ModelChoice{}, false
	}
	entries := CollectEntries(sess.Entries())
	if len(entries) == 0 {
		return ModelChoice{}, false
	}
	ctx := session.BuildSessionContext(entries, "")
	if ctx == nil || ctx.Model == nil {
		return ModelChoice{}, false
	}
	choice := ModelChoice{
		Provider: strings.TrimSpace(strings.ToLower(ctx.Model.Provider)),
		Name:     strings.TrimSpace(ctx.Model.ModelID),
	}
	if !choice.Valid() {
		return ModelChoice{}, false
	}
	return choice, true
}

func SortedPendingInterruptIDs(cp *session.Checkpoint) []string {
	if cp == nil || len(cp.PendingInterrupts) == 0 {
		return nil
	}
	ids := make([]string, 0, len(cp.PendingInterrupts))
	for id := range cp.PendingInterrupts {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

func (s SessionSummary) Detail() string {
	updated := "unknown"
	if !s.UpdatedAt.IsZero() {
		updated = s.UpdatedAt.Local().Format(time.DateTime)
	}
	if s.PendingInterrupt > 0 {
		return fmt.Sprintf("updated %s | pending approvals %d", updated, s.PendingInterrupt)
	}
	return fmt.Sprintf("updated %s", updated)
}

func approvalInfoFromPending(info any) (interrupttool.ApprovalInfo, bool) {
	switch typed := info.(type) {
	case interrupttool.ApprovalInfo:
		return typed, true
	case *interrupttool.ApprovalInfo:
		if typed == nil {
			return interrupttool.ApprovalInfo{}, false
		}
		return *typed, true
	case map[string]any:
		out := interrupttool.ApprovalInfo{}
		if toolName, ok := typed["tool"].(string); ok {
			out.Tool = toolName
		}
		if args, ok := typed["args"].(map[string]any); ok {
			out.Args = cloneAnyMap(args)
		}
		if message, ok := typed["message"].(string); ok {
			out.Message = message
		}
		if out.Tool == "" {
			return interrupttool.ApprovalInfo{}, false
		}
		return out, true
	default:
		return interrupttool.ApprovalInfo{}, false
	}
}

func cloneAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = cloneAnyValue(v)
	}
	return dst
}

func cloneAnyValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneAnyMap(typed)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = cloneAnyValue(item)
		}
		return out
	default:
		return value
	}
}

func previewText(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return string([]rune(s)[:maxLen])
	}
	return string([]rune(s)[:maxLen-3]) + "..."
}
