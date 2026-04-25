package resume

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// InterruptSignal marks a tool-driven pause that should be resumed later.
type InterruptSignal struct {
	ID      string             `json:"id"`
	Address string             `json:"address"`
	Info    any                `json:"info"`
	State   any                `json:"state"`
	Subs    []*InterruptSignal `json:"subs,omitempty"`
}

func (i *InterruptSignal) Error() string {
	if i == nil {
		return "interrupt signal"
	}
	if i.ID == "" {
		return "interrupt signal: <no id>"
	}
	return fmt.Sprintf("interrupt signal: %s", i.ID)
}

func (i *InterruptSignal) IsRootCause() bool {
	return i != nil && len(i.Subs) == 0
}

// ToPersistenceMaps flattens the interrupt signal tree for persistence.
func (i *InterruptSignal) ToPersistenceMaps() (address2id map[string]string, id2State map[string]any) {
	address2id = make(map[string]string)
	id2State = make(map[string]any)
	if i == nil {
		return address2id, id2State
	}
	var walk func(*InterruptSignal)
	walk = func(sig *InterruptSignal) {
		if sig == nil {
			return
		}
		address2id[sig.Address] = sig.ID
		id2State[sig.ID] = sig.State
		for _, sub := range sig.Subs {
			walk(sub)
		}
	}
	walk(i)
	return address2id, id2State
}

// Interrupt creates an interrupt signal without state.
func Interrupt(ctx context.Context, address string, info any) *InterruptSignal {
	return StatefulInterrupt(ctx, address, info, nil)
}

// StatefulInterrupt creates an interrupt signal with state.
func StatefulInterrupt(ctx context.Context, address string, info any, state any) *InterruptSignal {
	id := uuid.NewString()
	return &InterruptSignal{
		ID:      id,
		Address: address,
		Info:    info,
		State:   state,
	}
}

// CompositeInterrupt creates a parent interrupt signal from child signals.
func CompositeInterrupt(address string, info any, state any, subSig ...*InterruptSignal) *InterruptSignal {
	var subs []*InterruptSignal
	for _, sig := range subSig {
		if sig != nil {
			subs = append(subs, sig)
		}
	}
	return &InterruptSignal{
		ID:      uuid.NewString(),
		Address: address,
		Info:    info,
		State:   state,
		Subs:    subs,
	}
}

// IsInterrupt reports whether err is an InterruptSignal.
func IsInterrupt(err error) bool {
	var signal *InterruptSignal
	return errors.As(err, &signal)
}

// InterruptContext is a user-facing view of an interrupt.
type InterruptContext struct {
	ID          string            `json:"id"`
	Address     string            `json:"address"`
	Info        any               `json:"info"`
	IsRootCause bool              `json:"is_root_cause"`
	Parent      *InterruptContext `json:"parent,omitempty"`
}

// InterruptData stores persisted interrupt state for resume.
type InterruptData struct {
	Address2ID map[string]string   `json:"address_to_id"`
	ID2State   map[string]any      `json:"id_to_state"`
	Contexts   []*InterruptContext `json:"contexts,omitempty"`
}

// ToInterruptData converts an interrupt signal into a persistable structure.
func (i *InterruptSignal) ToInterruptData() *InterruptData {
	if i == nil {
		return &InterruptData{
			Address2ID: make(map[string]string),
			ID2State:   make(map[string]any),
		}
	}
	address2id, id2State := i.ToPersistenceMaps()
	return &InterruptData{
		Address2ID: address2id,
		ID2State:   id2State,
		Contexts:   i.toInterruptContexts(),
	}
}

func (id *InterruptData) HasInterrupt(address string) bool {
	_, exists := id.Address2ID[address]
	return exists
}

// ToSignals converts InterruptData back to a slice of top-level InterruptSignals.
// It reconstructs the tree structure from the parent chain.
func (id *InterruptData) ToSignals() []*InterruptSignal {
	if id == nil {
		return nil
	}

	// Collect all contexts (including parents) from the parent chains
	allContexts := make(map[string]*InterruptContext)
	for _, ctx := range id.Contexts {
		for c := ctx; c != nil; c = c.Parent {
			if _, exists := allContexts[c.ID]; !exists {
				allContexts[c.ID] = c
			}
		}
	}

	// Create signals for all contexts
	signals := make(map[string]*InterruptSignal)
	for _, ctx := range allContexts {
		sig := &InterruptSignal{
			ID:      ctx.ID,
			Address: ctx.Address,
			Info:    ctx.Info,
		}
		if state, ok := id.ID2State[ctx.ID]; ok {
			sig.State = state
		}
		signals[ctx.ID] = sig
	}

	// Build parent-child relationships (Subs) and find roots
	roots := make(map[string]*InterruptSignal)
	for _, ctx := range allContexts {
		sig := signals[ctx.ID]
		if ctx.Parent != nil {
			parentSig := signals[ctx.Parent.ID]
			parentSig.Subs = append(parentSig.Subs, sig)
		} else {
			roots[ctx.ID] = sig
		}
	}

	// Return root signals
	result := make([]*InterruptSignal, 0, len(roots))
	for _, sig := range roots {
		result = append(result, sig)
	}
	return result
}

func (i *InterruptSignal) toInterruptContexts() []*InterruptContext {
	if i == nil {
		return nil
	}
	var contexts []*InterruptContext
	var walk func(*InterruptSignal, *InterruptContext)
	walk = func(sig *InterruptSignal, parent *InterruptContext) {
		if sig == nil {
			return
		}
		ctx := &InterruptContext{
			ID:      sig.ID,
			Address: sig.Address,
			Info:    sig.Info,
			Parent:  parent,
		}
		if sig.IsRootCause() {
			ctx.IsRootCause = true
			contexts = append(contexts, ctx)
		}
		for _, sub := range sig.Subs {
			walk(sub, ctx)
		}
	}
	walk(i, nil)
	return contexts
}
