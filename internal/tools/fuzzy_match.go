package tools

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"go.uber.org/zap"
)

// MatchLevel 表示匹配级别
type MatchLevel int

const (
	// MatchExact 精确匹配
	MatchExact MatchLevel = iota
	// MatchLineTrimmed 按行 trim 首尾空白后匹配
	MatchLineTrimmed
	// MatchWhitespaceNormalized 将所有连续空白归一化为单空格后匹配
	MatchWhitespaceNormalized
	// MatchIndentationFlexible 忽略行首缩进差异，只匹配内容
	MatchIndentationFlexible
	// MatchBlockAnchor 首尾行锚点 + Levenshtein 相似度匹配
	MatchBlockAnchor
	// MatchEscapeNormalized 转义字符归一化后匹配
	MatchEscapeNormalized
	// MatchTrimmedBoundary 首尾行 trim 匹配，中间行精确匹配
	MatchTrimmedBoundary
	// MatchContextAware 首尾锚点 trim 匹配 + 中间行 50% 匹配
	MatchContextAware
	// MatchMultiOccurrence 允许多处精确匹配，返回第一处
	MatchMultiOccurrence
)

// matchLevelNames 匹配级别的中文名称（用于日志）
var matchLevelNames = map[MatchLevel]string{
	MatchExact:                "精确匹配",
	MatchLineTrimmed:          "行首尾空白容错匹配",
	MatchWhitespaceNormalized: "空白归一化匹配",
	MatchIndentationFlexible:  "缩进容错匹配",
	MatchBlockAnchor:          "首尾锚点相似度匹配",
	MatchEscapeNormalized:     "转义归一化匹配",
	MatchTrimmedBoundary:      "边界Trim匹配",
	MatchContextAware:         "上下文感知匹配",
	MatchMultiOccurrence:      "多处出现匹配",
}

// Replacer 定义模糊匹配器接口
type Replacer interface {
	// Match 在文件行中查找与 hunk 行匹配的位置
	// fileLines: 文件的所有行
	// hunkLines: hunk 中需要匹配的行（上下文行和删除行）
	// startHint: 建议的起始行位置（0-based），-1 表示无提示
	// 返回: startLine 匹配到的起始行（0-based），ok 是否匹配成功
	Match(fileLines []string, hunkLines []string, startHint int) (startLine int, ok bool)

	// Level 返回匹配级别
	Level() MatchLevel
}

// --- ExactReplacer: 精确匹配 ---

// ExactReplacer 精确匹配：要求文件行与 hunk 行完全一致
type ExactReplacer struct{}

func (r *ExactReplacer) Match(fileLines []string, hunkLines []string, startHint int) (int, bool) {
	if len(hunkLines) == 0 {
		// 无需匹配的行（纯添加 hunk），直接返回 hint 位置
		if startHint >= 0 && startHint <= len(fileLines) {
			return startHint, true
		}
		return 0, true
	}

	// 优先从 startHint 位置开始匹配
	if startHint >= 0 && startHint+len(hunkLines) <= len(fileLines) {
		if exactMatch(fileLines, hunkLines, startHint) {
			return startHint, true
		}
	}

	// 回退：在整个文件中搜索
	for i := 0; i+len(hunkLines) <= len(fileLines); i++ {
		if exactMatch(fileLines, hunkLines, i) {
			return i, true
		}
	}

	return -1, false
}

func (r *ExactReplacer) Level() MatchLevel { return MatchExact }

// exactMatch 检查从 start 位置开始是否精确匹配
func exactMatch(fileLines, hunkLines []string, start int) bool {
	for j, hl := range hunkLines {
		if fileLines[start+j] != hl {
			return false
		}
	}
	return true
}

// --- LineTrimmedReplacer: 按行 trim 首尾空白后匹配 ---

// LineTrimmedReplacer 按行 trim 首尾空白后匹配
type LineTrimmedReplacer struct{}

func (r *LineTrimmedReplacer) Match(fileLines []string, hunkLines []string, startHint int) (int, bool) {
	if len(hunkLines) == 0 {
		if startHint >= 0 && startHint <= len(fileLines) {
			return startHint, true
		}
		return 0, true
	}

	// 预计算 hunk 行的 trimmed 版本
	trimmedHunk := make([]string, len(hunkLines))
	for i, l := range hunkLines {
		trimmedHunk[i] = strings.TrimSpace(l)
	}

	// 优先从 startHint 位置开始匹配
	if startHint >= 0 && startHint+len(hunkLines) <= len(fileLines) {
		if trimmedMatch(fileLines, trimmedHunk, startHint) {
			return startHint, true
		}
	}

	// 回退：在整个文件中搜索
	for i := 0; i+len(hunkLines) <= len(fileLines); i++ {
		if trimmedMatch(fileLines, trimmedHunk, i) {
			return i, true
		}
	}

	return -1, false
}

func (r *LineTrimmedReplacer) Level() MatchLevel { return MatchLineTrimmed }

// trimmedMatch 检查从 start 位置开始，trim 后是否匹配
func trimmedMatch(fileLines, trimmedHunk []string, start int) bool {
	for j, th := range trimmedHunk {
		if strings.TrimSpace(fileLines[start+j]) != th {
			return false
		}
	}
	return true
}

// --- WhitespaceNormalizedReplacer: 将所有连续空白归一化为单空格后匹配 ---

// WhitespaceNormalizedReplacer 将所有连续空白归一化为单空格后匹配
type WhitespaceNormalizedReplacer struct{}

// 匹配连续空白字符的正则
var multiSpaceRE = regexp.MustCompile(`\s+`)

// normalizeWhitespace 将字符串中的连续空白归一化为单空格，并 trim 首尾
func normalizeWhitespace(s string) string {
	return strings.TrimSpace(multiSpaceRE.ReplaceAllString(s, " "))
}

func (r *WhitespaceNormalizedReplacer) Match(fileLines []string, hunkLines []string, startHint int) (int, bool) {
	if len(hunkLines) == 0 {
		if startHint >= 0 && startHint <= len(fileLines) {
			return startHint, true
		}
		return 0, true
	}

	// 预计算 hunk 行的归一化版本
	normalizedHunk := make([]string, len(hunkLines))
	for i, l := range hunkLines {
		normalizedHunk[i] = normalizeWhitespace(l)
	}

	// 优先从 startHint 位置开始匹配
	if startHint >= 0 && startHint+len(hunkLines) <= len(fileLines) {
		if normalizedMatch(fileLines, normalizedHunk, startHint) {
			return startHint, true
		}
	}

	// 回退：在整个文件中搜索
	for i := 0; i+len(hunkLines) <= len(fileLines); i++ {
		if normalizedMatch(fileLines, normalizedHunk, i) {
			return i, true
		}
	}

	return -1, false
}

func (r *WhitespaceNormalizedReplacer) Level() MatchLevel { return MatchWhitespaceNormalized }

// normalizedMatch 检查从 start 位置开始，归一化空白后是否匹配
func normalizedMatch(fileLines, normalizedHunk []string, start int) bool {
	for j, nh := range normalizedHunk {
		if normalizeWhitespace(fileLines[start+j]) != nh {
			return false
		}
	}
	return true
}

// --- IndentationFlexibleReplacer: 忽略行首缩进差异，只匹配内容 ---

// IndentationFlexibleReplacer 忽略行首缩进差异，只匹配非缩进内容
type IndentationFlexibleReplacer struct{}

func (r *IndentationFlexibleReplacer) Match(fileLines []string, hunkLines []string, startHint int) (int, bool) {
	if len(hunkLines) == 0 {
		if startHint >= 0 && startHint <= len(fileLines) {
			return startHint, true
		}
		return 0, true
	}

	// 预计算 hunk 行去除行首缩进后的内容
	strippedHunk := make([]string, len(hunkLines))
	for i, l := range hunkLines {
		strippedHunk[i] = strings.TrimLeft(l, " \t")
	}

	// 优先从 startHint 位置开始匹配
	if startHint >= 0 && startHint+len(hunkLines) <= len(fileLines) {
		if indentFlexMatch(fileLines, strippedHunk, startHint) {
			return startHint, true
		}
	}

	// 回退：在整个文件中搜索
	for i := 0; i+len(hunkLines) <= len(fileLines); i++ {
		if indentFlexMatch(fileLines, strippedHunk, i) {
			return i, true
		}
	}

	return -1, false
}

func (r *IndentationFlexibleReplacer) Level() MatchLevel { return MatchIndentationFlexible }

// indentFlexMatch 检查从 start 位置开始，忽略缩进后是否匹配
func indentFlexMatch(fileLines, strippedHunk []string, start int) bool {
	for j, sh := range strippedHunk {
		if strings.TrimLeft(fileLines[start+j], " \t") != sh {
			return false
		}
	}
	return true
}

// --- BlockAnchorReplacer: 首尾行锚点 + Levenshtein 相似度匹配 ---

// BlockAnchorReplacer 用首尾行作为锚点定位代码块，中间内容用 Levenshtein 相似度匹配
type BlockAnchorReplacer struct{}

// blockAnchorSimilarityThreshold 中间内容的 Levenshtein 相似度阈值
const blockAnchorSimilarityThreshold = 0.6

// blockAnchorMaxCompareLen Levenshtein 比较的最大字符长度（性能保护）
const blockAnchorMaxCompareLen = 10000

func (r *BlockAnchorReplacer) Match(fileLines []string, hunkLines []string, startHint int) (int, bool) {
	// 锚点匹配需要至少 3 行（首行 + 中间 + 尾行）
	if len(hunkLines) < 3 {
		return -1, false
	}

	firstAnchor := strings.TrimSpace(hunkLines[0])
	lastAnchor := strings.TrimSpace(hunkLines[len(hunkLines)-1])
	hunkMiddle := strings.Join(hunkLines[1:len(hunkLines)-1], "\n")

	// 限制中间内容比较长度
	if len(hunkMiddle) > blockAnchorMaxCompareLen {
		return -1, false
	}

	// 在文件中搜索首行锚点
	for i := 0; i+len(hunkLines) <= len(fileLines); i++ {
		if strings.TrimSpace(fileLines[i]) != firstAnchor {
			continue
		}

		// 找到首行锚点后，检查对应位置的尾行锚点
		endIdx := i + len(hunkLines) - 1
		if endIdx >= len(fileLines) {
			continue
		}
		if strings.TrimSpace(fileLines[endIdx]) != lastAnchor {
			continue
		}

		// 首尾锚点都匹配，计算中间内容的相似度
		fileMiddle := strings.Join(fileLines[i+1:endIdx], "\n")
		if len(fileMiddle) > blockAnchorMaxCompareLen {
			continue
		}

		_, above := levenshteinSimilarityAboveThreshold(hunkMiddle, fileMiddle, blockAnchorSimilarityThreshold)
		if above {
			return i, true
		}
	}

	return -1, false
}

func (r *BlockAnchorReplacer) Level() MatchLevel { return MatchBlockAnchor }

// levenshteinDistance 计算两个字符串的编辑距离（标准 DP 算法，两行数组优化内存）。
// maxDist 为提前终止阈值：当某行最小值已超过 maxDist 时提前返回。传 0 表示不限制。
func levenshteinDistance(a, b string, maxDist int) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	// 使用两行 DP 数组节省内存
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)

	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr[0] = i
		rowMin := curr[0]
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(
				prev[j]+1,      // 删除
				curr[j-1]+1,    // 插入
				prev[j-1]+cost, // 替换
			)
			if curr[j] < rowMin {
				rowMin = curr[j]
			}
		}
		// 提前终止：当前行最小值已超过阈值，后续只会更大
		if maxDist > 0 && rowMin > maxDist {
			return rowMin
		}
		prev, curr = curr, prev
	}

	return prev[lb]
}

// levenshteinSimilarity 计算两个字符串的相似度 (0.0 ~ 1.0)
func levenshteinSimilarity(a, b string) float64 {
	maxLen := max(len(a), len(b))
	if maxLen == 0 {
		return 1.0
	}
	dist := levenshteinDistance(a, b, 0)
	return 1.0 - float64(dist)/float64(maxLen)
}

// levenshteinSimilarityAboveThreshold 判断相似度是否 >= threshold，利用提前终止优化性能
func levenshteinSimilarityAboveThreshold(a, b string, threshold float64) (float64, bool) {
	maxLen := max(len(a), len(b))
	if maxLen == 0 {
		return 1.0, true
	}
	maxDist := int(float64(maxLen) * (1.0 - threshold))
	dist := levenshteinDistance(a, b, maxDist)
	sim := 1.0 - float64(dist)/float64(maxLen)
	return sim, sim >= threshold
}

// --- EscapeNormalizedReplacer: 转义字符归一化后匹配 ---

// EscapeNormalizedReplacer 归一化转义字符后匹配，处理 LLM 常见的转义差异
type EscapeNormalizedReplacer struct{}

func (r *EscapeNormalizedReplacer) Match(fileLines []string, hunkLines []string, startHint int) (int, bool) {
	if len(hunkLines) == 0 {
		if startHint >= 0 && startHint <= len(fileLines) {
			return startHint, true
		}
		return 0, true
	}

	// 预计算 hunk 行的转义归一化版本
	normalizedHunk := make([]string, len(hunkLines))
	for i, l := range hunkLines {
		normalizedHunk[i] = normalizeEscapes(l)
	}

	// 优先从 startHint 位置开始匹配
	if startHint >= 0 && startHint+len(hunkLines) <= len(fileLines) {
		if escapeNormalizedMatch(fileLines, normalizedHunk, startHint) {
			return startHint, true
		}
	}

	// 回退：在整个文件中搜索
	for i := 0; i+len(hunkLines) <= len(fileLines); i++ {
		if escapeNormalizedMatch(fileLines, normalizedHunk, i) {
			return i, true
		}
	}

	return -1, false
}

func (r *EscapeNormalizedReplacer) Level() MatchLevel { return MatchEscapeNormalized }

// escapeNormalizedMatch 检查从 start 位置开始，转义归一化后是否匹配
func escapeNormalizedMatch(fileLines, normalizedHunk []string, start int) bool {
	for j, nh := range normalizedHunk {
		if normalizeEscapes(fileLines[start+j]) != nh {
			return false
		}
	}
	return true
}

// normalizeEscapes 归一化字符串中的转义序列
// 将 literal backslash + 字符的组合转换为对应的控制字符
func normalizeEscapes(s string) string {
	var b strings.Builder
	b.Grow(len(s))

	i := 0
	for i < len(s) {
		if i+1 < len(s) && s[i] == '\\' {
			next := s[i+1]
			switch next {
			case 'n':
				b.WriteByte('\n')
				i += 2
				continue
			case 't':
				b.WriteByte('\t')
				i += 2
				continue
			case '"':
				b.WriteByte('"')
				i += 2
				continue
			case '\'':
				b.WriteByte('\'')
				i += 2
				continue
			case '\\':
				b.WriteByte('\\')
				i += 2
				continue
			case 'u':
				// Unicode 转义: \uXXXX
				if i+5 < len(s) {
					hex := s[i+2 : i+6]
					if isHexString(hex) {
						r := hexToRune(hex)
						b.WriteRune(r)
						i += 6
						continue
					}
				}
			}
		}
		b.WriteByte(s[i])
		i++
	}

	return b.String()
}

// isHexString 检查字符串是否为合法的 4 位十六进制
func isHexString(s string) bool {
	if len(s) != 4 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// hexToRune 将 4 位十六进制字符串转换为 rune，无效 Unicode 码点返回替换字符
func hexToRune(hex string) rune {
	var r rune
	for _, c := range hex {
		r <<= 4
		switch {
		case c >= '0' && c <= '9':
			r |= rune(c - '0')
		case c >= 'a' && c <= 'f':
			r |= rune(c-'a') + 10
		case c >= 'A' && c <= 'F':
			r |= rune(c-'A') + 10
		}
	}
	if !utf8.ValidRune(r) {
		return unicode.ReplacementChar
	}
	return r
}

// --- TrimmedBoundaryReplacer: 首尾行 trim 匹配，中间行精确匹配 ---

// TrimmedBoundaryReplacer 首尾行 TrimSpace 后匹配，中间行要求完全精确匹配。
// 适用于 LLM 只在首尾行添加了空白的场景。
type TrimmedBoundaryReplacer struct{}

func (r *TrimmedBoundaryReplacer) Match(fileLines []string, hunkLines []string, startHint int) (int, bool) {
	// 至少需要 2 行（首行 + 尾行）
	if len(hunkLines) < 2 {
		return -1, false
	}

	firstTrimmed := strings.TrimSpace(hunkLines[0])
	lastTrimmed := strings.TrimSpace(hunkLines[len(hunkLines)-1])

	// 在文件中搜索匹配位置
	for i := 0; i+len(hunkLines) <= len(fileLines); i++ {
		// 首行 trim 后匹配
		if strings.TrimSpace(fileLines[i]) != firstTrimmed {
			continue
		}

		// 尾行 trim 后匹配
		endIdx := i + len(hunkLines) - 1
		if strings.TrimSpace(fileLines[endIdx]) != lastTrimmed {
			continue
		}

		// 中间行精确匹配
		matched := true
		for j := 1; j < len(hunkLines)-1; j++ {
			if fileLines[i+j] != hunkLines[j] {
				matched = false
				break
			}
		}
		if matched {
			return i, true
		}
	}

	return -1, false
}

func (r *TrimmedBoundaryReplacer) Level() MatchLevel { return MatchTrimmedBoundary }

// --- ContextAwareReplacer: 首尾锚点 + 中间行 50% 匹配 ---

// ContextAwareReplacer 首尾行作为锚点精确 trim 匹配，中间内容只需 50% 行匹配即可。
// 比 BlockAnchorReplacer 更宽松的变体，适用于大段代码中有部分行被 LLM 修改的场景。
type ContextAwareReplacer struct{}

// contextAwareMiddleThreshold 中间行匹配比例阈值
const contextAwareMiddleThreshold = 0.5

func (r *ContextAwareReplacer) Match(fileLines []string, hunkLines []string, startHint int) (int, bool) {
	// 至少需要 3 行（首行 + 中间 + 尾行）
	if len(hunkLines) < 3 {
		return -1, false
	}

	firstTrimmed := strings.TrimSpace(hunkLines[0])
	lastTrimmed := strings.TrimSpace(hunkLines[len(hunkLines)-1])

	// 在文件中搜索匹配位置
	for i := 0; i+len(hunkLines) <= len(fileLines); i++ {
		// 首行 trim 后匹配
		if strings.TrimSpace(fileLines[i]) != firstTrimmed {
			continue
		}

		// 尾行 trim 后匹配
		endIdx := i + len(hunkLines) - 1
		if strings.TrimSpace(fileLines[endIdx]) != lastTrimmed {
			continue
		}

		// 中间行逐行 TrimSpace 后比较，统计匹配数
		middleCount := len(hunkLines) - 2
		matchedCount := 0
		for j := 1; j < len(hunkLines)-1; j++ {
			if strings.TrimSpace(fileLines[i+j]) == strings.TrimSpace(hunkLines[j]) {
				matchedCount++
			}
		}

		// 至少 50% 的中间行匹配
		if float64(matchedCount)/float64(middleCount) >= contextAwareMiddleThreshold {
			return i, true
		}
	}

	return -1, false
}

func (r *ContextAwareReplacer) Level() MatchLevel { return MatchContextAware }

// --- MultiOccurrenceReplacer: 允许多处精确匹配，返回第一处 ---

// MultiOccurrenceReplacer 精确匹配 hunkLines 在 fileLines 中的位置。
// 与 ExactReplacer 的区别：ExactReplacer 在 FuzzyFindString 中要求唯一匹配，
// 此 replacer 允许文件中存在多处匹配，返回第一处的起始行号。
type MultiOccurrenceReplacer struct{}

func (r *MultiOccurrenceReplacer) Match(fileLines []string, hunkLines []string, startHint int) (int, bool) {
	if len(hunkLines) == 0 {
		if startHint >= 0 && startHint <= len(fileLines) {
			return startHint, true
		}
		return 0, true
	}

	// 在整个文件中搜索第一处精确匹配
	for i := 0; i+len(hunkLines) <= len(fileLines); i++ {
		if exactMatch(fileLines, hunkLines, i) {
			return i, true
		}
	}

	return -1, false
}

func (r *MultiOccurrenceReplacer) Level() MatchLevel { return MatchMultiOccurrence }

// --- FuzzyFindString: 字符串级别的模糊查找 ---

// FuzzyFindString 在 content 中模糊查找 oldString，返回找到的精确匹配字符串
// 策略：精确 → trim 行首尾 → 空白归一化 → 缩进弹性 → 首尾锚点相似度 → 转义归一化 → 边界trim → 上下文感知 → 多处出现
// 返回：找到的原始文件中的字符串(用于替换)，使用的匹配级别，是否找到
func FuzzyFindString(content, oldString string, logger *zap.Logger) (foundString string, level MatchLevel, ok bool) {
	// 精确匹配直接返回
	if strings.Contains(content, oldString) {
		return oldString, MatchExact, true
	}

	// 将 content 和 oldString 按行分割
	fileLines := strings.Split(content, "\n")
	oldLines := strings.Split(oldString, "\n")

	if len(oldLines) == 0 {
		return "", 0, false
	}

	// 依次尝试各级模糊匹配器（跳过精确匹配，已尝试过）
	replacers := defaultReplacers()
	for _, r := range replacers {
		if r.Level() == MatchExact {
			continue
		}

		startLine, matched := r.Match(fileLines, oldLines, -1)
		if matched {
			// 提取文件中实际匹配位置的原始行
			matchedLines := fileLines[startLine : startLine+len(oldLines)]
			foundStr := strings.Join(matchedLines, "\n")

			logger.Debug("edit 使用模糊匹配",
				zap.String("级别", matchLevelNames[r.Level()]),
				zap.Int("匹配起始行", startLine+1),
			)

			return foundStr, r.Level(), true
		}
	}

	return "", 0, false
}

// --- 渐进匹配调度 ---

// defaultReplacers 返回默认的 9 级渐进匹配器列表
func defaultReplacers() []Replacer {
	return []Replacer{
		&ExactReplacer{},
		&LineTrimmedReplacer{},
		&WhitespaceNormalizedReplacer{},
		&IndentationFlexibleReplacer{},
		&BlockAnchorReplacer{},
		&EscapeNormalizedReplacer{},
		&TrimmedBoundaryReplacer{},
		&ContextAwareReplacer{},
		&MultiOccurrenceReplacer{},
	}
}

// fuzzyMatchResult 模糊匹配结果
type fuzzyMatchResult struct {
	StartLine  int        // 匹配到的起始行（0-based）
	MatchLevel MatchLevel // 使用的匹配级别
}

// fuzzyMatchHunk 使用渐进匹配策略在文件行中查找 hunk 对应的位置
// fileLines: 文件所有行
// hunk: 要匹配的 hunk
// reverse: 是否反向应用
// logger: 日志记录器
// 返回匹配结果或错误
func fuzzyMatchHunk(fileLines []string, hunk *Hunk, reverse bool, logger *zap.Logger) (*fuzzyMatchResult, error) {
	// 提取 hunk 中需要匹配文件内容的行（上下文行 + 删除行）
	var matchLines []string
	for _, line := range hunk.Lines {
		lineType := line.Type
		if reverse {
			if lineType == LineAdded {
				lineType = LineRemoved
			} else if lineType == LineRemoved {
				lineType = LineAdded
			}
		}

		switch lineType {
		case LineContext, LineRemoved:
			matchLines = append(matchLines, line.Content)
		}
	}

	// 计算起始位置提示
	startHint := hunk.OldStart - 1 // 转换为 0-based
	if reverse {
		startHint = hunk.NewStart - 1
	}

	// 依次尝试各级匹配器
	replacers := defaultReplacers()
	for _, r := range replacers {
		startLine, ok := r.Match(fileLines, matchLines, startHint)
		if ok {
			level := r.Level()
			if level > MatchExact {
				logger.Debug("hunk 使用容错匹配",
					zap.String("级别", matchLevelNames[level]),
					zap.Int("匹配起始行", startLine+1),
				)
			}
			return &fuzzyMatchResult{
				StartLine:  startLine,
				MatchLevel: level,
			}, nil
		}
	}

	// 所有匹配器都失败，返回详细错误信息
	// 用原始 startHint 位置给出期望 vs 实际的差异提示
	if startHint >= 0 && startHint < len(fileLines) && len(matchLines) > 0 {
		expected := matchLines[0]
		actual := fileLines[startHint]
		return nil, fmt.Errorf("所有匹配策略均失败：行 %d 期望 %q，实际 %q", startHint+1, expected, actual)
	}

	return nil, fmt.Errorf("所有匹配策略均失败：无法在文件中找到匹配的内容")
}
