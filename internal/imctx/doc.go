// Package imctx 是所有 IM 通道与 master 共用的"中立"消息上下文叶子包。
//
// 不变量（Phase 0 P0-#1，CI 强制执行）：
//
//  1. 仅依赖标准库——禁止 import 任何 github.com/chef-guo/agents-hive/internal/* 包。
//     违反将打破 master ↔ channel 的解耦，重新引入潜在循环依赖。
//  2. 类型零方法依赖——本包不应有"行为"，只承载"数据"。业务方法（鉴权、转换、分发）
//     一律放在调用方的包里实现，imctx 类型只暴露字段与极少量构造器。
//  3. 字段命名应当 IM 通用，不绑定单一平台。Feishu 专属字段统一前缀 Feishu*；
//     若字段对所有 IM 都成立（如 TenantKey/ChannelMessageID）则不加前缀。
//
// 加入新字段前请先回答：channel/* 与 master/* 是否都要消费？只有 channel 层需要的
// 应留在 channel/<platform> 包内，避免本包膨胀成"上帝结构体"。
package imctx
