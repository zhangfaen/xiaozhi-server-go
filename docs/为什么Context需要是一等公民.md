# 为什么 Context 需要是一等公民

本文面向对 Go（Golang）不太熟、但在做后端/服务端工程的读者，目标是把 `context.Context` 的**定位、价值、最佳实践**讲清楚，并给出一套可落地的“循序渐进”改造思路（尤其适用于 WebSocket + 语音流式处理这类长连接场景）。

> 一句话：把 `Context` 当成“一等公民”，意味着 **取消、超时、以及请求级元信息**必须像业务参数一样被显式传递、贯穿调用链，并且被每一层尊重。

---

## 1. Context 到底是什么（工程视角）

在 Go 里，`context.Context` 主要承担三件事：

1) **取消信号（Cancellation）**  
上游发出“别做了”的信号（连接断开、用户打断、请求被取消、服务关闭等），下游应尽快停止阻塞、停止重试、停止继续工作并返回。

2) **截止时间/超时（Deadline / Timeout）**  
给操作一个明确的时间边界：到了就取消，避免“卡死”或“永远重试”拖垮资源。

3) **请求级元信息（Request-scoped metadata）**  
例如 `request_id`、trace id、session id 等，用于日志/链路追踪的关联（通过 `context.Value`，但应克制使用）。

`Context` 的关键特征：

- 它是**只读信号**：下游只能观察 `Done()`/`Deadline()`/`Value()`，不能“反向修改上游”。
- 它是**可传播**的：通过 `context.WithCancel/WithTimeout/WithDeadline` 派生子 context，构成清晰的生命周期树。
- 它是**生命周期的表达**：一个 `Context` 对应一段工作“应该活多久、何时必须停”。

---

## 2. 为什么要把 Context 当成“一等公民”

很多系统在早期能跑，但随着业务变复杂，最常见的稳定性问题往往不是算法，而是**生命周期不可控**。当 `Context` 不是一等公民时，典型后果如下。

### 2.1 取消不生效：Stop/Close 只能取消“一部分”

常见反模式：在业务链路中途使用 `context.Background()` 重新开始一条链。

这样做的含义是：**你主动切断了父子关系**。上游即使已经取消（连接断开、用户 stop），下游仍会继续：

- 继续网络请求/重试
- 继续占用 goroutine
- 继续写队列/写文件
- 继续占用 provider 连接/资源池

结果就是：你以为“停了”，实际上只是“你看不到了”。

### 2.2 超时不统一：每层自设/不设，问题难排查

没有贯穿的 ctx，往往会出现：

- 某些操作永不超时（偶发卡死，复现难）
- 某些 provider 内部自带超时，但上层无法统一配置与观察
- 一次请求超时后，后台仍在重试或继续跑（形成资源毛刺）

### 2.3 可观测性断链：traceID/sessionID 传不下去

`Context` 是贯穿调用链的天然载体（尤其在 HTTP handler、RPC、WebSocket 会话里）。一旦中途用 `Background()` 断开，日志关联与链路追踪会断，排障成本陡增。

---

## 3. Context 的“正确心智模型”

可以用一句话建立直觉：

> **生命周期由最上游拥有，并通过 Context 向下游传播；下游必须尊重它。**

这意味着两点：

- 上游对“什么时候必须停”负责（连接关闭、用户打断、服务退出）。
- 下游对“如何尽快停、如何释放资源”负责（停止阻塞、停止重试、关闭连接、退出 goroutine）。

---

## 4. 最佳实践（Do / Don’t）

### 4.1 Do：把 ctx 作为函数签名的“第一参数”

惯例：

- `func Do(ctx context.Context, ...) ...`
- ctx 不要藏在全局变量里；不要让调用链靠“隐式状态”传递取消/超时。

### 4.2 Do：谁启动 goroutine，谁保证它能退出

如果你 `go func(){...}()`，你要能回答：

- 它什么时候退出？
- 上游取消时它会不会继续跑？

最常见的退出方式就是：

- `select { case <-ctx.Done(): return ... }`

### 4.3 Do：在阻塞点尊重 ctx（真正能“立刻停”）

取消要想“立刻”生效，必须穿透到真正的阻塞点，比如：

- `http.NewRequestWithContext(ctx, ...)`
- `DialContext(ctx, ...)`
- 等待队列/Channel 的地方用 `select` 监听 `<-ctx.Done()`
- 重试循环里每次迭代都检查 `ctx.Done()`

### 4.4 Don’t：在请求链路里随意使用 `context.Background()`

`Background()` 的语义是“没有父亲、永不取消”。它适合：

- 进程启动时的根 context
- 与任何请求无关的长期后台任务（并且通常仍应可被程序退出信号取消）

它不适合：

- Web 请求处理链路
- WebSocket 会话链路
- 一次对话/一次语音识别链路

### 4.5 Don’t：滥用 `context.Value`

`Value` 应仅用于**请求级元信息**，并且要：

- 使用自定义 key 类型（避免冲突）
- 不放大对象、不放业务实体、不当“万能参数”

---

## 5. 长连接/语音链路：如何划分 Context 的层级

语音系统（WebSocket + 音频帧 + ASR 流式连接）最容易踩坑：一边是实时音频流，一边是 provider 的长连接/重试/缓冲。要想做到 `listen.stop` “立刻停”，必须先把生命周期划清楚。

推荐三层（从粗到细）：

### 5.1 连接级：`connCtx`

- 生命周期：WebSocket 连接建立 → 连接关闭
- 用途：连接关闭时，所有与该连接相关的 goroutine、ASR/TTS 处理都必须结束

### 5.2 监听会话级：`listenCtx`

- 生命周期：收到 `listen.start` → 收到 `listen.stop`（或 abort/连接关闭）
- 用途：表达“正在采集并识别用户语音”的区间
- 目标：`listen.stop` 时，**停止继续喂音频**，并取消在途网络阻塞/重试

### 5.3 单次发言/单次识别级：`utteranceCtx`（可选，进阶）

- 生命周期：一次发言开始 → ASR final / silence timeout / stop
- 用途：更精细地控制“一句”的超时与打断（例如 realtime 模式下每句话一个 ctx）

在“循序渐进”的路线里，你可以先做 `connCtx + listenCtx`，就能解决很多“stop 不立刻停”的问题；`utteranceCtx` 是后续优化的空间。

---

## 6. “逐步让 ctx 变成一等公民”：一条低风险演进路径

现实项目往往是存量代码 + 多 provider 实现，不可能一刀切。所谓“逐步”，核心是：

1) **调用侧先变更**：优先在上层把 ctx 传下去，即使接口暂时没显式暴露。
2) **实现侧再补齐**：逐个 provider 支持 ctx（尤其是连接建立、写入、重试循环）。
3) **最后升级接口契约**：当大多数实现就绪后，把 ctx 纳入接口，形成硬约束。

### 6.1 调用侧的兼容技巧：能力探测（type assertion）

当接口还只有 `AddAudio([]byte)`，但部分实现已经支持 `AddAudioWithContext(ctx, ...)` 时，可以在调用侧做“能力探测”：

- 如果实现提供 `AddAudioWithContext`：用 ctx 版本（可取消、可超时）
- 否则：退回旧版本（保持兼容）

这样可以在不破坏现有代码的前提下，让“支持 ctx 的 provider”马上受益。

---

## 7. 如何判断你做对了（可验证的工程标准）

当你把 `Context` 当成一等公民，你应该能明确回答这些问题：

1) **用户 stop/连接关闭后，ASR 最多多久一定停？**  
如果答案是“看运气/看 provider”，通常就是 ctx 没贯穿或下游没尊重。

2) **有没有无界后台工作？**  
比如 goroutine 不退出、队列持续堆积、provider 长连接不关、重试不止。

3) **一次 listen 会话是否有清晰边界？**  
start/stop 之间喂音频；stop 后不再喂，并尽快结束在途工作。

---

## 8. 结合本项目的语音链路（落地示例）

以“先保证 `listen.stop` 立刻停”为目标，本项目采取的最小落地策略是：

- 引入 `listening` 闸门：不在 listen 会话中时，直接忽略音频帧（避免 stop 后还继续堆积/继续喂给 ASR）。
- 引入 `listenCtx`：`listen.start` 创建、`listen.stop` 取消。
- 喂音频时优先调用 `AddAudioWithContext(listenCtx, data)`（如实现支持），否则退回 `AddAudio(data)`。
- `listen.stop` 额外清空音频队列（避免 stop 前积压的帧在 stop 后继续被消费）。

这套策略的价值在于：

- 改动小、风险低、可逐步推进
- 先解决最痛点：stop 后继续喂音频导致“停不下来”
- 为下一步“取消 provider 内部网络阻塞/重试”铺好路（只要 provider 支持 ctx，就能被 listenCtx 取消）

---

## 9. 下一步建议（在 stop 立刻停之后）

当你已经做到 stop 后不再喂音频，下一步通常是：

1) **让 provider 的“长连接/重试循环”真正可取消**  
把 ctx 传到 `DialContext`、读写循环、重试 backoff 等关键路径。

2) **统一 Reset/CloseConnection 的语义**  
在 stop 场景下，你可能既想“尽快停”，又想“尽量拿到 final”。通常需要一个明确策略：  
例如 stop 触发“最后一帧 + 短暂 grace window”，超时后强制取消 ctx 并关闭连接。

3) **收敛状态控制**  
把散落的 `bool/atomic/chan` 控制逐步收敛：能用 ctx 表达的优先用 ctx，剩下少量状态再用原子变量/锁。

---

## 10. 一页 Checklist（写代码时快速自检）

- 这个函数是否应该接收 `ctx context.Context`？
- 是否在关键阻塞点（网络/队列/重试）监听了 `<-ctx.Done()`？
- 是否在请求链路里使用了 `context.Background()`（如果是，是否真的想“永不取消”）？
- goroutine 是否有明确退出路径？
- stop/close 时是否能保证“最多多久一定停”？
- `context.Value` 是否只放了少量请求级元信息（且 key 有类型）？

---

如果你希望把这篇文档继续“写得更贴近本项目”，可以在文档末尾追加一节：用项目中的消息（`listen.start/stop`）和协程/队列（音频队列、ASR 写入、ASR 长连接）画一张时序图，把“边界在哪里、ctx 怎么流动”直观展示出来。*** End Patch
