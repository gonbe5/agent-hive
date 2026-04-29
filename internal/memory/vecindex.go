package memory

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// 编译期接口合规检查
var _ VectorStore = (*VecIndex)(nil)

// VecIndex 内存中的向量索引，支持暴力余弦相似度搜索
// 向量数据持久化到 memories.embedding BLOB 列
// 实现 VectorStore 接口（InMemoryVecStore）
type VecIndex struct {
	mu      sync.RWMutex
	vectors map[int64][]float32 // memoryID → embedding vector
	dim     int                 // 向量维度
	logger  *zap.Logger
}

// NewVecIndex 创建空的向量索引
func NewVecIndex(dim int, logger *zap.Logger) *VecIndex {
	return &VecIndex{
		vectors: make(map[int64][]float32),
		dim:     dim,
		logger:  logger,
	}
}

// Add 添加或更新向量
func (v *VecIndex) Add(_ context.Context, id int64, vec []float32) error {
	if len(vec) == 0 {
		return nil
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	// 维度验证：已建立的维度必须与新向量一致
	if v.dim > 0 && len(vec) != v.dim {
		return fmt.Errorf("向量维度不匹配：期望 %d，得到 %d", v.dim, len(vec))
	}
	v.vectors[id] = vec
	if v.dim == 0 {
		v.dim = len(vec)
	}
	return nil
}

// Remove 删除向量
func (v *VecIndex) Remove(_ context.Context, id int64) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.vectors, id)
	return nil
}

// Search 暴力余弦相似度搜索，返回 TopK 最相似的结果
// userID 参数接收但忽略：内存索引无 owner 元数据，用户隔离由 HybridSearcher 层的 FTS 结果交叉过滤保证
func (v *VecIndex) Search(_ context.Context, query []float32, topK int, _ string) ([]VecSearchResult, error) {
	if len(query) == 0 || topK <= 0 {
		return nil, nil
	}

	v.mu.RLock()
	defer v.mu.RUnlock()

	if len(v.vectors) == 0 {
		return nil, nil
	}

	// 计算所有向量的余弦相似度
	results := make([]VecSearchResult, 0, len(v.vectors))
	for id, vec := range v.vectors {
		score := cosineSimilarity(query, vec)
		results = append(results, VecSearchResult{ID: id, Score: score})
	}

	// 按相似度降序排序
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > topK {
		results = results[:topK]
	}

	return results, nil
}

// Count 返回索引中的向量数量
func (v *VecIndex) Count(_ context.Context) (int, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return len(v.vectors), nil
}

// Size 返回索引中的向量数量（向后兼容快捷方法）
func (v *VecIndex) Size() int {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return len(v.vectors)
}

// Close 释放资源（内存实现无需操作）
func (v *VecIndex) Close() error {
	return nil
}

// LoadFromPool 从 PostgreSQL 加载所有已有的 embedding 向量到内存
func (v *VecIndex) LoadFromPool(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, embedding FROM memories WHERE embedding IS NOT NULL`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	loaded := 0
	for rows.Next() {
		var id int64
		var blob []byte
		if err := rows.Scan(&id, &blob); err != nil {
			continue
		}

		vec := decodeFloat32s(blob)
		if len(vec) > 0 {
			_ = v.Add(ctx, id, vec)
			loaded++
		}
	}

	if loaded > 0 && v.logger != nil {
		v.logger.Info("从 PostgreSQL 加载向量索引",
			zap.Int("loaded", loaded),
			zap.Int("dim", v.dim),
		)
	}

	return loaded, nil
}

// cosineSimilarity 计算两个向量的余弦相似度
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return dot / denom
}

// encodeFloat32s 将 float32 切片编码为二进制（小端序）
func encodeFloat32s(vec []float32) []byte {
	buf := make([]byte, len(vec)*4)
	for i, v := range vec {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}

// decodeFloat32s 从二进制解码 float32 切片（小端序）
func decodeFloat32s(data []byte) []float32 {
	if len(data)%4 != 0 {
		return nil
	}
	vec := make([]float32, len(data)/4)
	for i := range vec {
		vec[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
	}
	return vec
}
