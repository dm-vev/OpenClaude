package tools

import (
	"context"
	"sync"
)

// TaskManager tracks running task cancellation hooks.
type TaskManager struct {
	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

// NewTaskManager constructs an empty task manager.
func NewTaskManager() *TaskManager {
	return &TaskManager{
		cancels: map[string]context.CancelFunc{},
	}
}

// Register associates a task id with its cancel function.
func (m *TaskManager) Register(taskID string, cancel context.CancelFunc) {
	if m == nil || taskID == "" || cancel == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cancels[taskID] = cancel
}

// Unregister removes a task id from the manager.
func (m *TaskManager) Unregister(taskID string) {
	if m == nil || taskID == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.cancels, taskID)
}

// Cancel stops a running task when possible.
func (m *TaskManager) Cancel(taskID string) bool {
	if m == nil || taskID == "" {
		return false
	}
	m.mu.Lock()
	cancel, ok := m.cancels[taskID]
	if ok {
		delete(m.cancels, taskID)
	}
	m.mu.Unlock()
	if !ok {
		return false
	}
	cancel()
	return true
}
