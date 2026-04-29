package memory

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// 编译期接口合规检查
var _ VectorStore = (*PgVectorStore)(nil)

// PgVectorStore 基于 pgvector 的向量索引实现
// 使用 HNSW 索引实现 O(log n) 近似最近邻搜索
type PgVectorStore struct {
	pool *pgxpool.Pool
	// dim 记录已建立的向量维度，用于检测多 provider 混用导致的不一致维度问题
	// 通过 dimOnce 保证并发安全（write-once-then-read-many 模式）
	dim     int
	dimOnce sync.Once
	logger  *zap.Logger
}

// NewPgVectorStore 创建 pgvector 向量索引
func NewPgVectorStore(pool *pgxpool.Pool, logger *zap.Logger) *PgVectorStore {
	return &PgVectorStore{pool: pool, logger: logger}
}

// newPgVectorStoreWithDim 创建预设维度的 pgvector 向量索引（仅测试用）
func newPgVectorStoreWithDim(dim int) *PgVectorStore {
	s := &PgVectorStore{dim: dim}
	if dim > 0 {
		s.dimOnce.Do(func() {}) // 标记维度已设置
	}
	return s
}

// Add 添加或更新向量（写入 memories.embedding_vec 列）
func (s *PgVectorStore) Add(ctx context.Context, id int64, vec []float32) error {
	if len(vec) == 0 {
		return nil
	}
	// 维度一致性检查（并发安全：首次写入通过 dimOnce 保护）
	var dimErr error
	s.dimOnce.Do(func() {
		s.dim = len(vec)
	})
	if s.dim != len(vec) {
		dimErr = fmt.Errorf("向量维度不匹配：已建立维度 %d，新向量维度 %d", s.dim, len(vec))
	}
	if dimErr != nil {
		return dimErr
	}
	vecStr := float32sToVecLiteral(vec)
	_, err := s.pool.Exec(ctx,
		`UPDATE memories SET embedding_vec = $1::vector WHERE id = $2`,
		vecStr, id)
	return err
}

// Remove 删除向量（置空 embedding_vec 列）
func (s *PgVectorStore) Remove(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE memories SET embedding_vec = NULL WHERE id = $1`, id)
	return err
}

// Search 使用 pgvector 余弦距离搜索最相似的 topK 个向量
func (s *PgVectorStore) Search(ctx context.Context, query []float32, topK int, userID string) ([]VecSearchResult, error) {
	if len(query) == 0 || topK <= 0 {
		return nil, nil
	}
	if dim := s.dim; dim > 0 && len(query) != dim {
		return nil, fmt.Errorf("查询向量维度不匹配：已建立维度 %d，查询维度 %d", dim, len(query))
	}
	vecStr := float32sToVecLiteral(query)
	var rows pgx.Rows
	var err error
	if userID != "" {
		rows, err = s.pool.Query(ctx, `
			SELECT id, 1 - (embedding_vec <=> $1::vector) AS score
			FROM memories
			WHERE embedding_vec IS NOT NULL AND user_id = $3
			ORDER BY embedding_vec <=> $1::vector
			LIMIT $2
		`, vecStr, topK, userID)
	} else {
		rows, err = s.pool.Query(ctx, `
			SELECT id, 1 - (embedding_vec <=> $1::vector) AS score
			FROM memories
			WHERE embedding_vec IS NOT NULL
			ORDER BY embedding_vec <=> $1::vector
			LIMIT $2
		`, vecStr, topK)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []VecSearchResult
	for rows.Next() {
		var r VecSearchResult
		if err := rows.Scan(&r.ID, &r.Score); err != nil {
			continue
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// Count 返回已索引的向量数量
func (s *PgVectorStore) Count(ctx context.Context) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM memories WHERE embedding_vec IS NOT NULL`).Scan(&count)
	return count, err
}

// Close 释放资源（不关闭共享连接池）
func (s *PgVectorStore) Close() error {
	return nil
}

// float32sToVecLiteral 将 float32 切片转换为 pgvector 字面量格式 "[1.0,2.0,3.0]"
// 使用 %.8e 格式保证 8 位有效数字，足以无损 round-trip float32（约 7 位精度）
func float32sToVecLiteral(vec []float32) string {
	parts := make([]string, len(vec))
	for i, v := range vec {
		parts[i] = fmt.Sprintf("%.8e", v)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

