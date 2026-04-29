package explore

// ExploreRequest 是代码探索任务的输入
type ExploreRequest struct {
	// Instruction 是统一输入协议字段，优先于 Target
	Instruction string `json:"instruction,omitempty"`
	Target      string `json:"target"`          // 探索目标（目录路径、特定模块等）
	Focus       string `json:"focus,omitempty"` // 聚焦领域（"architecture", "api", "dependencies"等）
	Depth       string `json:"depth,omitempty"` // 探索深度（"quick", "normal", "deep"）
}

// effectiveTarget 返回实际使用的探索目标：优先 Instruction，其次 Target
func (r ExploreRequest) effectiveTarget() string {
	if r.Instruction != "" {
		return r.Instruction
	}
	return r.Target
}

// ExploreOutput Explore Agent 的输出结构
type ExploreOutput struct {
	Summary   string           `json:"summary"`    // 探索摘要
	Structure ProjectStructure `json:"structure"`  // 项目结构
	KeyFiles  []KeyFile        `json:"key_files"`  // 关键文件
	Patterns  []CodePattern    `json:"patterns"`   // 代码模式
	Insights  []string         `json:"insights"`   // 发现和洞察
}

// ProjectStructure 描述项目的目录结构
type ProjectStructure struct {
	RootPath    string            `json:"root_path"`
	Directories map[string]string `json:"directories"` // path -> description
	FileTypes   map[string]int    `json:"file_types"`  // extension -> count
}

// KeyFile 描述关键文件及其重要性
type KeyFile struct {
	Path       string `json:"path"`
	Purpose    string `json:"purpose"`
	Importance string `json:"importance"` // "high", "medium", "low"
}

// CodePattern 描述发现的代码模式
type CodePattern struct {
	Pattern     string   `json:"pattern"`
	Description string   `json:"description"`
	Examples    []string `json:"examples"`
}
