// Package assistantcap 提供 P0-A structural lock 的 capability token。
//
// 设计目标：把"必须在 evaluateRequiredGuard pass 之后才能 emit assistant 消息"
// 从静态 AST 检查升级为编译期类型不变量。
//
// Capability 的 unexported field 使其在本包外无法用 composite literal 构造，
// 唯一构造入口是 Grant* 函数。这些函数把 gate 通过条件作为前置 guard，
// 不通过则返回零值 Capability + ok=false。
//
// 在 master 包内，所有写入 Role:"assistant" 或 payload role:"assistant"
// 的代码路径必须接受 Capability 参数；无 Capability 的路径在运行时 panic。
//
// 红方任何 IIFE/shadow/Builder/factory/reflect/unsafe/cgo/init 形态：
//   - 跨包想构造 Capability{} → 编译失败（unexported field）
//   - 同包内伪造 Grant* 调用 → AST 测试规则限定 Grant* 的 guard 形态
//   - 直接调 master.appendSessionMessage(Role:"assistant") → 运行时 panic
package assistantcap

// Capability 是不可伪造的 P0-A emit 授权。
//
// 设计成 interface 而非 struct 是因为 Go spec 允许任意包对零值 struct 用空
// composite literal `pkg.S{}` 构造（即使带 unexported field），所以 struct
// 形态做不到 cross-package 编译期不可伪造。改为 interface + unexported method
// 后，跨包代码无法构造任何实现 Capability 的具体类型 —— 编译器报
// `does not implement assistantcap.Capability (missing assistantcap method)`。
//
// 唯一能持有 Capability 的具体值就是本包内私有类型 grant{}，且只通过
// Grant* 函数颁发；外部任何 IIFE/shadow/Builder/factory/reflect/unsafe 都
// 拿不到合法 Capability 值。
type Capability interface {
	// assistantcap 是 unexported sentinel method，跨包类型无法实现，
	// 形成 compile-time unforgeability 边界。
	assistantcap()
}

// grant 是 Capability 的唯一私有实现。零字节。
// 因为 assistantcap() 是 unexported method，外部包无法定义满足 Capability 的类型。
type grant struct{}

func (grant) assistantcap() {}

// granted 是预颁发的 sentinel 单例，避免每次 Grant* 都堆分配。
// 仅在 Grant* gate 通过时返回 granted；否则返回 nil interface。
var granted Capability = grant{}

// GrantPass 在 gate-pass 后颁发 Capability。
// 调用方必须把 evaluateRequiredGuard 返回的 action 与 requiredGuardPass 常量值传入。
// 用 int 而非具体 enum 类型是为了避免循环 import（master 的 enum 在 master 包内）。
func GrantPass(action int, passValue int) (Capability, bool) {
	if action != passValue {
		return nil, false
	}
	return granted, true
}

// GrantStream 在 tool_choice != "required" 时颁发 Capability，覆盖流 partial 广播。
// tool_choice == required 时流 partial 必须被抑制（shouldSuppressStreamPartial）。
func GrantStream(toolChoice string, requiredValue string) (Capability, bool) {
	if toolChoice == requiredValue {
		return nil, false
	}
	return granted, true
}
