package memory

import "context"

// VectorStore 可插拔向量索引接口
// 实现：InMemoryVecStore（VecIndex）、PgVectorStore（pgvector HNSW）
type VectorStore interface {
	// Add 添加或更新向量
	Add(ctx context.Context, id int64, vec []float32) error

	// Remove 删除向量
	Remove(ctx context.Context, id int64) error

	// Search 搜索最相似的 topK 个向量，userID 非空时只返回该用户的结果
	Search(ctx context.Context, query []float32, topK int, userID string) ([]VecSearchResult, error)

	// Count 返回索引中的向量数量
	Count(ctx context.Context) (int, error)

	// Close 释放资源
	Close() error
}

// VecSearchResult 向量搜索结果
type VecSearchResult struct {
	ID    int64
	Score float64 // 余弦相似度，范围 [-1, 1]，越大越相似
}
