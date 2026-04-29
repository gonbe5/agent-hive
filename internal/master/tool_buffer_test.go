package master

import (
	"testing"

	"github.com/chef-guo/agents-hive/internal/mcphost"
)

func TestOrderedBuffer_Add_InOrder(t *testing.T) {
	buf := NewOrderedBuffer(16)

	// 按顺序添加，结果立即 emit
	r1 := &mcphost.ToolResult{Content: []byte(`"r1"`)}
	r2 := &mcphost.ToolResult{Content: []byte(`"r2"`)}

	batch1 := buf.Add(r1)
	if batch1 == nil {
		t.Fatal("expected batch1, got nil")
	}
	if len(batch1) != 1 || batch1[0] != r1 {
		t.Errorf("batch1 wrong: %v", batch1)
	}

	batch2 := buf.Add(r2)
	if batch2 == nil {
		t.Fatal("expected batch2, got nil")
	}
	if len(batch2) != 1 || batch2[0] != r2 {
		t.Errorf("batch2 wrong: %v", batch2)
	}
}

func TestOrderedBuffer_Add_OutOfOrder(t *testing.T) {
	buf := NewOrderedBuffer(16)

	// r1 先到但等待（未立即 emit）
	r1 := &mcphost.ToolResult{Content: []byte(`"r1"`)}
	r2 := &mcphost.ToolResult{Content: []byte(`"r2"`)}

	// r1 先添加（nextEmit=0, idx=0, match → emit）
	batch1 := buf.Add(r1)
	if batch1 == nil {
		t.Fatal("expected batch1, got nil")
	}

	// r2 后添加（nextEmit=1, idx=1, match → emit）
	batch2 := buf.Add(r2)
	if batch2 == nil {
		t.Fatal("expected batch2, got nil")
	}

	// 验证顺序
	if batch1[0] != r1 || batch2[0] != r2 {
		t.Errorf("order wrong: r1=%v r2=%v", batch1, batch2)
	}
}

// P0-2 回归测试：空切片时 Add 不会越界
func TestOrderedBuffer_Add_EmptySlice(t *testing.T) {
	buf := NewOrderedBuffer(16)
	r := &mcphost.ToolResult{Content: []byte(`"test"`)}
	batch := buf.Add(r)
	if batch == nil {
		t.Error("first Add on empty buffer should emit immediately")
	}
	if len(batch) != 1 || batch[0] != r {
		t.Errorf("wrong batch: %v", batch)
	}
}

func TestOrderedBuffer_NewWithZeroSize(t *testing.T) {
	buf := NewOrderedBuffer(0)
	if buf == nil {
		t.Fatal("NewOrderedBuffer(0) should not return nil")
	}
	r := &mcphost.ToolResult{Content: []byte(`"x"`)}
	batch := buf.Add(r)
	if batch == nil {
		t.Error("Add should work with zero-size buffer")
	}
}
