package master

import (
	"errors"
	"fmt"
	"maps"
	"reflect"
	"regexp"
	"sort"
	"sync"
)

// cloneSpec 深拷贝 ChoiceTypeSpec，切断 PayloadHint map 的别名，防止外部修改污染 registry。
func cloneSpec(s ChoiceTypeSpec) ChoiceTypeSpec {
	s.PayloadHint = maps.Clone(s.PayloadHint)
	return s
}

// ChoiceTypeSpec 描述一个业务决策 HITL 子语义。
//
// Name 必须与 InputRequest.ChoiceType 匹配；PayloadHint 是开发者文档性提示，
// 不做强类型校验——业务层自行保证 InputRequest.Data 的 JSON 结构对齐。
type ChoiceTypeSpec struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	PayloadHint map[string]string `json:"payload_hint,omitempty"`
}

var (
	choiceTypeNameRE = regexp.MustCompile(`^[a-z][a-z0-9_]+$`)

	choiceTypeMu       sync.RWMutex
	choiceTypeRegistry = map[string]ChoiceTypeSpec{}

	ErrChoiceTypeNameInvalid                = errors.New("choice_type name must match ^[a-z][a-z0-9_]+$")
	ErrChoiceTypeAlreadyRegisteredDifferent = errors.New("choice_type already registered with a different spec")
	ErrUnregisteredChoiceType               = errors.New("choice_type is not registered in choice_type_registry")
)

// RegisterChoiceType 线程安全地注册一个业务决策类型。
//   - 名字非法 → ErrChoiceTypeNameInvalid
//   - 同名且 spec 相同 → 幂等返回 nil（支持 Skill 热重载）
//   - 同名但 spec 不同 → ErrChoiceTypeAlreadyRegisteredDifferent
func RegisterChoiceType(spec ChoiceTypeSpec) error {
	if !choiceTypeNameRE.MatchString(spec.Name) {
		return fmt.Errorf("%w: %q", ErrChoiceTypeNameInvalid, spec.Name)
	}
	choiceTypeMu.Lock()
	defer choiceTypeMu.Unlock()
	if existing, ok := choiceTypeRegistry[spec.Name]; ok {
		if reflect.DeepEqual(existing, spec) {
			return nil
		}
		return fmt.Errorf("%w: %q", ErrChoiceTypeAlreadyRegisteredDifferent, spec.Name)
	}
	choiceTypeRegistry[spec.Name] = cloneSpec(spec)
	return nil
}

// MustRegisterChoiceType 是 init() 专用：失败直接 panic。
func MustRegisterChoiceType(spec ChoiceTypeSpec) {
	if err := RegisterChoiceType(spec); err != nil {
		panic(fmt.Sprintf("MustRegisterChoiceType(%q): %v", spec.Name, err))
	}
}

// IsRegisteredChoiceType 返回 name 是否已注册。空字符串返回 false。
func IsRegisteredChoiceType(name string) bool {
	if name == "" {
		return false
	}
	choiceTypeMu.RLock()
	defer choiceTypeMu.RUnlock()
	_, ok := choiceTypeRegistry[name]
	return ok
}

// ListChoiceTypes 返回按 Name 字典序排序的快照副本（含 PayloadHint 深拷贝）。
// 调用方可随意修改返回值，不会影响 registry 内部状态。
func ListChoiceTypes() []ChoiceTypeSpec {
	choiceTypeMu.RLock()
	out := make([]ChoiceTypeSpec, 0, len(choiceTypeRegistry))
	for _, spec := range choiceTypeRegistry {
		out = append(out, cloneSpec(spec))
	}
	choiceTypeMu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func init() {
	MustRegisterChoiceType(ChoiceTypeSpec{
		Name:        "account_selector",
		Description: "User selects which upstream account to use for a multi-account skill",
	})
	MustRegisterChoiceType(ChoiceTypeSpec{
		Name:        "ambiguity_clarification",
		Description: "Master detected intent ambiguity and asks user to disambiguate",
	})
	MustRegisterChoiceType(ChoiceTypeSpec{
		Name:        "confirmation_before_irreversible_business_action",
		Description: "Skill/Tool asks for explicit confirmation before irreversible external side-effects",
	})
}
