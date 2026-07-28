package qa

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/codebase-copilot/core/internal/domain"
	"github.com/codebase-copilot/core/internal/embedding"
	"github.com/codebase-copilot/core/internal/vectorstore"
)

// Service defines the QA interface consumed by HTTP handlers.
type Service interface {
	Ask(ctx context.Context, repoID, question string, history []domain.Message, w http.ResponseWriter) (*AskResult, error)
}

// AskResult holds the accumulated result of a RAG query for persistence.
type AskResult struct {
	FullText   string
	Citations  []domain.Citation
	Confidence float64
	Tokens     int
}

// RAGService orchestrates the full RAG pipeline: embed, search, expand, rerank, assemble, and stream.
type RAGService struct {
	emb      *embedding.Client
	searcher *vectorstore.Searcher
	llm      *LLMClient
}

// NewRAGService creates a new RAG service.
func NewRAGService(emb *embedding.Client, searcher *vectorstore.Searcher, llm *LLMClient) *RAGService {
	return &RAGService{emb: emb, searcher: searcher, llm: llm}
}

// Ask runs the full RAG pipeline and streams the answer via SSE to the writer.
// Returns an AskResult with the accumulated response for persistence.
func (s *RAGService) Ask(ctx context.Context, repoID, question string, history []domain.Message, w http.ResponseWriter) (*AskResult, error) {
	// Step 1: Embed the question
	vecs, err := s.emb.Embed(ctx, []string{question})
	if err != nil {
		return nil, fmt.Errorf("embed question: %w", err)
	}

	// Step 2: Semantic search → top 50
	results, err := s.searcher.SemanticSearch(ctx, repoID, vecs[0], 50)
	if err != nil {
		return nil, fmt.Errorf("semantic search: %w", err)
	}

	// Step 3: Graph expand → ±1 hop call edges
	seedIDs := make([]string, len(results))
	for i, r := range results {
		seedIDs[i] = r.Node.ID
	}
	expanded, _ := s.searcher.GraphExpand(ctx, repoID, seedIDs)
	// Merge expanded into results with a lower score
	for _, n := range expanded {
		results = append(results, vectorstore.SearchResult{Node: n, Score: 0.3})
	}

	// Step 4: Dedup by node ID
	seen := make(map[string]bool)
	var unique []vectorstore.SearchResult
	for _, r := range results {
		if !seen[r.Node.ID] {
			seen[r.Node.ID] = true
			unique = append(unique, r)
		}
	}

	// Step 5: Rerank to top-10
	docs := make([]string, len(unique))
	for i, r := range unique {
		docs[i] = fmt.Sprintf("[%s:%d] %s\n%s", r.Node.FilePath, r.Node.StartLine, r.Node.Signature, r.Node.Code)
	}
	reranked, err := s.emb.Rerank(ctx, question, docs, 10)
	if err != nil {
		// Fallback: use top-10 from semantic search
		reranked = make([]embedding.RerankResult, 0)
		for i := 0; i < len(unique) && i < 10; i++ {
			reranked = append(reranked, embedding.RerankResult{Index: i, Score: float32(unique[i].Score)})
		}
	}

	// Step 6: Build context from top results
	var contextParts []string
	var citations []domain.Citation
	for _, rr := range reranked {
		if rr.Index < len(unique) {
			n := unique[rr.Index].Node
			contextParts = append(contextParts, fmt.Sprintf(
				"File: %s (line %d-%d)\n%s\n```\n%s\n```",
				n.FilePath, n.StartLine, n.EndLine, n.Signature, n.Code,
			))
			citations = append(citations, domain.Citation{
				File:    n.FilePath,
				Line:    n.StartLine,
				Content: n.Code,
				Score:   float64(rr.Score),
			})
		}
	}
	contextBlock := fmt.Sprintf("You are analyzing a code repository. Use the following code snippets to answer the question.\n\nCODE:\n%s\n\n---\n", join(contextParts, "\n\n"))

	// Step 7: Build messages for LLM
	systemMsg := ChatMessage{Role: "system", Content: "You are an expert code analyst. Answer questions based on the provided code snippets. Be specific, reference file paths and line numbers when citing code. If the provided context isn't sufficient, say so."}
	userMsg := ChatMessage{Role: "user", Content: contextBlock + "\n\nQuestion: " + question}

	// Insert conversation history in chronological order (simplified: last 6 messages)
	var historyMsgs []ChatMessage
	start := len(history) - 6
	if start < 0 {
		start = 0
	}
	for i := start; i < len(history); i++ {
		historyMsgs = append(historyMsgs, ChatMessage{Role: history[i].Role, Content: history[i].Content})
	}
	// Insert history after system message, before user context
	messages := append([]ChatMessage{systemMsg}, append(historyMsgs, userMsg)...)

	// Step 8: Setup SSE writer
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("streaming not supported")
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Step 9: Stream LLM response → SSE chunks, accumulating full text
	var fullText strings.Builder
	totalTokens := 0
	err = s.llm.StreamChat(ctx, messages, func(text string) {
		fullText.WriteString(text)
		chunk := domain.SSEChunk{
			Text:      text,
			Citations: citations, // Send citations with first chunk
		}
		data, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "event: chunk\ndata: %s\n\n", data)
		flusher.Flush()
		totalTokens++
	})
	if err != nil {
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
		flusher.Flush()
		return nil, err
	}

	// Step 10: Send done event
	done := domain.SSEDone{
		Confidence: 0.85, // TODO: calculate from rerank scores
		Tokens:     totalTokens,
	}
	data, _ := json.Marshal(done)
	fmt.Fprintf(w, "event: done\ndata: %s\n\n", data)
	flusher.Flush()

	result := &AskResult{
		FullText:   fullText.String(),
		Citations:  citations,
		Confidence: 0.85,
		Tokens:     totalTokens,
	}
	return result, nil
}

func join(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += sep + parts[i]
	}
	return result
}
