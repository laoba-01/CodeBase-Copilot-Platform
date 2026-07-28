package domain

type IndexNodeType string

const (
	NodeTypeFile     IndexNodeType = "file"
	NodeTypeFunction IndexNodeType = "function"
	NodeTypeClass    IndexNodeType = "class"
	NodeTypeMethod   IndexNodeType = "method"
)

type IndexNode struct {
	ID        string        `json:"id"`
	RepoID    string        `json:"repo_id"`
	Type      IndexNodeType `json:"type"`
	Name      string        `json:"name"`
	Signature string        `json:"signature"`       // 函数签名
	Code      string        `json:"code"`            // 原始代码
	FilePath  string        `json:"file_path"`
	StartLine int           `json:"start_line"`
	EndLine   int           `json:"end_line"`
	Summary   string        `json:"summary"`     // LLM 生成的一句话描述
	Embedding []float32     `json:"-"`           // 不序列化
	Language  string        `json:"language"`
	Package   string        `json:"package"`     // Go package / JS module
}

type CallEdge struct {
	ID       string `json:"id"`
	RepoID   string `json:"repo_id"`
	CallerID string `json:"caller_id"`  // → IndexNode.ID
	CalleeID string `json:"callee_id"`  // → IndexNode.ID
	FilePath string `json:"file_path"`
	Line     int    `json:"line"`
}

type DepEdge struct {
	ID       string `json:"id"`
	RepoID   string `json:"repo_id"`
	SourceID string `json:"source_id"`  // → IndexNode.ID (file)
	TargetID string `json:"target_id"`  // → IndexNode.ID (file)
	DepType  string `json:"dep_type"`   // import, extend, implement
}
