// Package asyncop tracks ARM long-running operations. Work is applied to
// the store synchronously (so state is deterministic), but the operation
// reports InProgress until its configured delay elapses, which exercises
// the client's Azure-AsyncOperation / Location polling exactly like Azure.
package asyncop

import (
	"strings"
	"sync"
	"time"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/entra"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/store"
)

// Operation is a long-running operation record.
type Operation struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	SubscriptionID string         `json:"subscriptionId"`
	ResourceID     string         `json:"resourceId"`
	Status         string         `json:"status"` // final status once DoneAt has passed
	StartTime      time.Time      `json:"startTime"`
	DoneAt         time.Time      `json:"doneAt"`
	Error          map[string]any `json:"error,omitempty"`
}

// Manager stores operations.
type Manager struct {
	st    store.Store
	mu    sync.Mutex
	delay func() time.Duration
}

// New creates a manager. delay is consulted for every new operation so
// chaos settings can slow operations down at runtime.
func New(st store.Store, delay func() time.Duration) *Manager {
	return &Manager{st: st, delay: delay}
}

// Start records a finished unit of work as an operation.
// status should be "Succeeded", "Failed" or "Canceled".
func (m *Manager) Start(subscriptionID, resourceID, status string, opErr map[string]any) Operation {
	now := time.Now().UTC()
	id := entra.NewGUID()
	op := Operation{ID: id, Name: id, SubscriptionID: subscriptionID, ResourceID: resourceID, Status: status, StartTime: now, DoneAt: now.Add(m.delay()), Error: opErr}
	m.mu.Lock()
	defer m.mu.Unlock()
	_ = store.PutJSON(m.st, store.BucketOps, strings.ToLower(id), op)
	return op
}

// Get loads an operation.
func (m *Manager) Get(id string) (Operation, bool) {
	var op Operation
	ok, err := store.GetJSON(m.st, store.BucketOps, strings.ToLower(id), &op)
	return op, ok && err == nil
}

// Done reports whether the operation has completed.
func (op Operation) Done() bool { return !time.Now().Before(op.DoneAt) }

// CurrentStatus is InProgress until done, then the final status.
func (op Operation) CurrentStatus() string {
	if !op.Done() {
		return "InProgress"
	}
	return op.Status
}

// StatusBody returns the Azure-AsyncOperation response body.
func (op Operation) StatusBody() map[string]any {
	b := map[string]any{
		"id":        op.ID,
		"name":      op.Name,
		"status":    op.CurrentStatus(),
		"startTime": op.StartTime.Format(time.RFC3339Nano),
	}
	if op.Done() {
		b["endTime"] = op.DoneAt.Format(time.RFC3339Nano)
		if op.Error != nil {
			b["error"] = op.Error
		}
	}
	return b
}
