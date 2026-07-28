package domain

type Question struct {
	RepoID         string `json:"repo_id" binding:"required"`
	Question       string `json:"question" binding:"required"`
	ConversationID string `json:"conversation_id,omitempty"`
}

type Citation struct {
	File    string  `json:"file"`
	Line    int     `json:"line"`
	Content string  `json:"content,omitempty"`
	Score   float64 `json:"score"`
}

type SSEChunk struct {
	Text      string     `json:"text"`
	Citations []Citation `json:"citations,omitempty"`
}

type SSEDone struct {
	Confidence float64 `json:"confidence"`
	Tokens     int     `json:"tokens"`
	ConvID     string  `json:"conv_id"`
}

type Conversation struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	RepoID    string `json:"repo_id"`
	Title     string `json:"title"`
	CreatedAt string `json:"created_at"`
}

type Message struct {
	ID        string     `json:"id"`
	ConvID    string     `json:"conv_id"`
	Role      string     `json:"role"`      // user / assistant
	Content   string     `json:"content"`
	Citations []Citation `json:"citations,omitempty"`
	Tokens    int        `json:"tokens"`
	CreatedAt string     `json:"created_at"`
}
