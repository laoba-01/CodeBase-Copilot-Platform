package domain

import "time"

type User struct {
	ID        string    `json:"id"`
	GitHubID  int64     `json:"github_id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	AvatarURL string    `json:"avatar_url"`
	CreatedAt time.Time `json:"created_at"`
}
