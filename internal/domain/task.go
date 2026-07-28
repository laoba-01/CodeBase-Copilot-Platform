package domain

import "time"

type TaskType string

const (
	TaskTypeIndex   TaskType = "index"
	TaskTypeReindex TaskType = "reindex"
)

type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"
	TaskStatusProcessing TaskStatus = "processing"
	TaskStatusCompleted  TaskStatus = "completed"
	TaskStatusFailed     TaskStatus = "failed"
)

type Task struct {
	ID        string     `json:"id"`
	RepoID    string     `json:"repo_id"`
	Type      TaskType   `json:"type"`
	Status    TaskStatus `json:"status"`
	Progress  int        `json:"progress"`   // 0-100
	Error     string     `json:"error,omitempty"`
	Result    string     `json:"result,omitempty"`  // JSON
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}
