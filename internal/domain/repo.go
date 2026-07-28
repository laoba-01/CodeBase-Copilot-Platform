package domain

import "time"

type RepoStatus string

const (
	RepoStatusPending  RepoStatus = "pending"
	RepoStatusCloning  RepoStatus = "cloning"
	RepoStatusIndexing RepoStatus = "indexing"
	RepoStatusReady    RepoStatus = "ready"
	RepoStatusError    RepoStatus = "error"
)

type Repository struct {
	ID            string     `json:"id"`
	UserID        string     `json:"user_id"`
	Name          string     `json:"name"`
	FullName      string     `json:"full_name"`    // owner/repo
	CloneURL      string     `json:"clone_url"`
	DefaultBranch string     `json:"default_branch"`
	Status        RepoStatus `json:"status"`
	IndexedAt     *time.Time `json:"indexed_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}
