package channel

import "unicode/utf8"

// ChunkText 按字节安全分块，不截断 UTF-8 字符
// maxBytes 为每块最大字节数
func ChunkText(text string, maxBytes int) []string {
	if maxBytes <= 0 {
		return []string{text}
	}
	if len(text) <= maxBytes {
		return []string{text}
	}

	var chunks []string
	for len(text) > 0 {
		if len(text) <= maxBytes {
			chunks = append(chunks, text)
			break
		}
		// 从 maxBytes 位置向前找到完整的 UTF-8 字符边界
		end := maxBytes
		for end > 0 && !utf8.RuneStart(text[end]) {
			end--
		}
		if end == 0 {
			end = maxBytes // 极端情况：没有有效 UTF-8 边界
		}
		chunks = append(chunks, text[:end])
		text = text[end:]
	}
	return chunks
}
