package embedding

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/codebase-copilot/core/proto/embedding"
)

type Client struct {
	conn   *grpc.ClientConn
	client pb.EmbeddingServiceClient
}

func NewClient(ctx context.Context, addr string) (*Client, error) {
	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithTimeout(10*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("dial embedding service: %w", err)
	}
	return &Client{
		conn:   conn,
		client: pb.NewEmbeddingServiceClient(conn),
	}, nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	resp, err := c.client.Embed(ctx, &pb.EmbedRequest{Texts: texts})
	if err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}
	vectors := make([][]float32, len(resp.Vectors))
	for i, v := range resp.Vectors {
		vectors[i] = v.Values
	}
	return vectors, nil
}

type RerankResult struct {
	Index int
	Score float32
}

func (c *Client) Rerank(ctx context.Context, query string, documents []string, topK int) ([]RerankResult, error) {
	resp, err := c.client.Rerank(ctx, &pb.RerankRequest{
		Query:     query,
		Documents: documents,
		TopK:      int32(topK),
	})
	if err != nil {
		return nil, fmt.Errorf("rerank: %w", err)
	}
	results := make([]RerankResult, len(resp.Results))
	for i, r := range resp.Results {
		results[i] = RerankResult{Index: int(r.Index), Score: r.Score}
	}
	return results, nil
}
