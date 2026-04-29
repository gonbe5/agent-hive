package memory

import (
	"context"
	"math"
	"strconv"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestVecIndex_AddAndCount(t *testing.T) {
	vi := NewVecIndex(3, zap.NewNop())
	ctx := context.Background()

	cnt, err := vi.Count(ctx)
	if err != nil || cnt != 0 {
		t.Fatalf("空索引 Count 应为 0, got %d, err %v", cnt, err)
	}

	if err := vi.Add(ctx, 1, []float32{1, 0, 0}); err != nil {
		t.Fatal(err)
	}
	if err := vi.Add(ctx, 2, []float32{0, 1, 0}); err != nil {
		t.Fatal(err)
	}

	cnt, _ = vi.Count(ctx)
	if cnt != 2 {
		t.Fatalf("Count 应为 2, got %d", cnt)
	}

	// 空 vec 不应添加
	if err := vi.Add(ctx, 3, nil); err != nil {
		t.Fatal(err)
	}
	cnt, _ = vi.Count(ctx)
	if cnt != 2 {
		t.Fatalf("空 vec 不应增加计数, got %d", cnt)
	}
}

func TestVecIndex_Remove(t *testing.T) {
	vi := NewVecIndex(3, zap.NewNop())
	ctx := context.Background()

	_ = vi.Add(ctx, 1, []float32{1, 0, 0})
	_ = vi.Add(ctx, 2, []float32{0, 1, 0})

	if err := vi.Remove(ctx, 1); err != nil {
		t.Fatal(err)
	}
	cnt, _ := vi.Count(ctx)
	if cnt != 1 {
		t.Fatalf("Remove 后 Count 应为 1, got %d", cnt)
	}

	// 删除不存在的 ID 不应报错
	if err := vi.Remove(ctx, 999); err != nil {
		t.Fatalf("删除不存在的 ID 不应报错, got %v", err)
	}
}

func TestVecIndex_Search(t *testing.T) {
	vi := NewVecIndex(3, zap.NewNop())
	ctx := context.Background()

	// 空索引搜索
	results, err := vi.Search(ctx, []float32{1, 0, 0}, 5, "")
	if err != nil || len(results) != 0 {
		t.Fatalf("空索引搜索应返回空, got %d results, err %v", len(results), err)
	}

	// 空 query
	results, err = vi.Search(ctx, nil, 5, "")
	if err != nil || results != nil {
		t.Fatalf("空 query 应返回 nil, got %v, err %v", results, err)
	}

	// topK <= 0
	results, err = vi.Search(ctx, []float32{1, 0, 0}, 0, "")
	if err != nil || results != nil {
		t.Fatalf("topK=0 应返回 nil, got %v", results)
	}

	// 正常搜索
	_ = vi.Add(ctx, 1, []float32{1, 0, 0})
	_ = vi.Add(ctx, 2, []float32{0, 1, 0})
	_ = vi.Add(ctx, 3, []float32{0.9, 0.1, 0})

	results, err = vi.Search(ctx, []float32{1, 0, 0}, 2, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("应返回 2 个结果, got %d", len(results))
	}

	// 第一个结果应是 ID=1（完全匹配）
	if results[0].ID != 1 {
		t.Errorf("第一个结果应为 ID=1, got %d", results[0].ID)
	}
	if math.Abs(results[0].Score-1.0) > 1e-6 {
		t.Errorf("完全匹配的 score 应为 1.0, got %f", results[0].Score)
	}

	// 第二个应是 ID=3（0.9 余弦相似度高于 ID=2）
	if results[1].ID != 3 {
		t.Errorf("第二个结果应为 ID=3, got %d", results[1].ID)
	}
}

func TestVecIndex_Close(t *testing.T) {
	vi := NewVecIndex(3, zap.NewNop())
	if err := vi.Close(); err != nil {
		t.Fatalf("Close 应返回 nil, got %v", err)
	}
}

func TestVecIndex_SizeBackCompat(t *testing.T) {
	vi := NewVecIndex(3, zap.NewNop())
	ctx := context.Background()

	_ = vi.Add(ctx, 1, []float32{1, 0, 0})
	if vi.Size() != 1 {
		t.Fatalf("Size 应为 1, got %d", vi.Size())
	}
}

func TestFloat32sToVecLiteral(t *testing.T) {
	tests := []struct {
		name string
		vec  []float32
		want string
	}{
		{"空", nil, "[]"},
		{"单值", []float32{1.5}, "[1.50000000e+00]"},
		{"多值", []float32{1, 2.5, 3}, "[1.00000000e+00,2.50000000e+00,3.00000000e+00]"},
		{"负数", []float32{-0.5, 0, 0.5}, "[-5.00000000e-01,0.00000000e+00,5.00000000e-01]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := float32sToVecLiteral(tt.vec)
			if got != tt.want {
				t.Errorf("float32sToVecLiteral(%v) = %q, want %q", tt.vec, got, tt.want)
			}
		})
	}
}

func TestFloat32sToVecLiteral_Precision(t *testing.T) {
	// 验证 %.8e 格式能无损 round-trip float32（约 7 位十进制精度）
	vec := []float32{
		0.123456791,
		1.23456788,
		3.14159274,
		0.100000001,
		-0.987654321,
		1e-7,
	}
	lit := float32sToVecLiteral(vec)
	inner := strings.TrimPrefix(strings.TrimSuffix(lit, "]"), "[")
	parts := strings.Split(inner, ",")
	if len(parts) != len(vec) {
		t.Fatalf("解析分量数量不匹配：got %d, want %d", len(parts), len(vec))
	}
	for i := range vec {
		parsed, err := strconv.ParseFloat(parts[i], 64)
		if err != nil {
			t.Fatalf("解析失败 [%s]: %v", parts[i], err)
		}
		if math.Abs(float64(vec[i])-parsed) > 1e-6 {
			t.Errorf("精度丢失: 原始 %.10f -> 解析 %.10f, 误差 %.2e",
				float64(vec[i]), parsed, math.Abs(float64(vec[i])-parsed))
		}
	}
}

func TestPgVectorStore_DimensionMismatch(t *testing.T) {
	// PgVectorStore.pool 是 *pgxpool.Pool（具体类型），无法 mock
	// 这里直接测试维度检查逻辑：dim 字段非零时，不同维度的 Add/Search 应返回错误
	s := newPgVectorStoreWithDim(3)

	// 不同维度应报错（维度检查先于 pool 调用，不会 panic）
	err := s.Add(context.Background(), 2, []float32{1, 0})
	if err == nil {
		t.Errorf("不同维度 Add 应返回错误")
	}

	// 空向量不报错
	err = s.Add(context.Background(), 3, nil)
	if err != nil {
		t.Errorf("空向量 Add 不应报错: %v", err)
	}

	// Search 查询维度不一致也应报错
	_, err = s.Search(context.Background(), []float32{1, 0}, 5, "")
	if err == nil {
		t.Errorf("Search 查询维度不一致应返回错误")
	}

	// 维度为 0 时首次 Add 应设置维度（但会在 pool.Exec 处 panic，所以只测 dim=0 + 空向量）
	s2 := newPgVectorStoreWithDim(0)
	err = s2.Add(context.Background(), 4, nil)
	if err != nil {
		t.Errorf("dim=0 空向量 Add 不应报错: %v", err)
	}
}
