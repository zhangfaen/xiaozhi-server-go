# 架构概览

## 客户端连接处理

| 组件 | 文件 | 说明 |
|------|------|------|
| WebSocket 入口 | `src/core/transport/websocket/transport.go` | 监听 WebSocket 连接 |
| 连接处理 | `src/core/connection.go` | `ConnectionHandler` 处理所有客户端交互 |
| Provider 池 | `src/core/pool/manager.go` | 池化管理 ASR/LLM/TTS Provider |

### 客户端标识获取 (`transport.go:112-127`)
```go
deviceID := r.Header.Get("Device-Id")
clientID := r.Header.Get("Client-Id")
// 如果没有提供，使用 conn 的内存地址作为 fallback
```

## 连接生命周期
```
main.go.StartTransportServer()
  → TransportManager.StartAll()
  → WebSocketTransport.Start()
  → handleWebSocket()
  → DefaultConnectionHandlerFactory.CreateHandler()
  → ConnectionContextAdapter.Handle(conn)
  → ConnectionHandler.Handle(conn)
```

### 一个客户端连接后的信息流故事

想象一下，用户对着玩具说："今天天气怎么样？"

1. **连接建立**
   - 用户设备通过 WebSocket 连接到服务器
   - 服务器从请求头中读取 `Device-Id` 和 `Client-Id`，为此次连接创建唯一的 `sessionID`
   - 从 Provider 池中获取一组 Provider（ASR/LLM/TTS），绑定到此 ConnectionHandler

2. **音频接收与识别**
   - 客户端发送 Opus 编码的音频数据
   - `handleListenMessage()` 接收音频，通过 `opusDecoder` 解码
   - 解码后的音频发送给 ASR Provider 进行语音识别
   - ASR 通过回调将识别结果（文本）返回给 ConnectionHandler

3. **对话历史记录**
   - ASR 返回的文本作为用户消息，存入 `dialogueManager`
   - 此时对话历史：`[{role: "user", content: "今天天气怎么样？"}]`

4. **LLM 推理**
   - `genResponseByLLM()` 从 `dialogueManager` 获取完整对话历史
   - 将历史消息发送给 LLM Provider
   - LLM 根据上下文理解意图，生成回复文本："今天天气晴朗，25度..."

5. **语音合成**
   - LLM 的文本回复存入对话历史
   - 文本发送给 TTS Provider，合成语音
   - 语音数据通过 WebSocket 发送给客户端，客户端播放

6. **多轮对话延续**
   - 用户再次提问："那明天呢？"
   - 重复步骤 2-5，此时对话历史已包含：
     ```
     [{role: "user", content: "今天天气怎么样？"},
      {role: "assistant", content: "今天天气晴朗，25度..."},
      {role: "user", content: "那明天呢？"}]
     ```
   - LLM 根据完整的上下文，理解"明天"指代"明天的天气"

**关键设计点**：
- 每个连接有独立的 `ConnectionHandler`，持有自己的 `dialogueManager`
- 对话历史在内存中管理，每个请求发送给 LLM 时传递完整历史
- Provider 从池中获取，连接结束后归还池中复用
- 无状态的 Provider + 有状态的 ConnectionHandler，既保证了资源复用，又支持多轮对话

### ConnectionHandler 结构 (`connection.go:68-155`)

> **说明**：以下为核心字段简化版，实际还有 `authManager`、`taskMgr`、`mcpManager`、`functionRegister`、`opusDecoder` 等字段。

```go
type ConnectionHandler struct {
    // 标识相关
    sessionID     string  // 服务端会话ID
    deviceID      string  // 设备ID
    clientId      string  // 客户端ID
    transportType string  // 传输类型

    // 核心组件
    dialogueManager *dialogue.Manager  // 对话历史管理
    providers       ProviderSet        // Provider 池（嵌套结构体：asr/llm/tts/vlllm）
    mcpManager     *mcp.Manager       // MCP 工具管理器
    functionRegister *function.FunctionRegistry  // 函数注册表

    // 音频相关
    opusDecoder *opus.Decoder  // Opus 解码器
    clientAudioFormat    string  // 客户端音频格式
    clientAudioSampleRate int    // 客户端采样率

    // 会话相关
    headers     http.Header     // 请求头
    agentID     string          // Agent ID
    enabledTools []string       // 启用的工具列表
}

## 消息处理 (`connection_handlemsg.go:15-100`)

| 消息类型 | 处理函数 | 说明 |
|----------|----------|------|
| `hello` | `handleHelloMessage()` | 初始化音频参数 |
| `chat` | `handleChatMessage()` | 文本聊天 |
| `listen` | `handleListenMessage()` | 语音识别控制 |
| `image` | `handleImageMessage()` | 图片消息 |
| `mcp` | `handleMCPResultCall()` | MCP 工具调用 |
| `abort` | `clientAbortChat()` | 中断当前对话 |

## Provider 调用方式

**Provider 是池化的、无状态的**，每个连接从池中获取独立的 `ProviderSet`（嵌套结构体）。

| Provider | 调用方式 | 说明 |
|----------|----------|------|
| LLM | `h.providers.llm.ResponseWithFunctions(ctx, sessionID, messages, tools)` | 无状态，通过 sessionID 关联 |
| ASR | `h.providers.asr.AddAudio()` + `SetListener(h)` | 需要设置回调绑定到 ConnectionHandler |
| TTS | `h.providers.tts.ToTTS(text)` | 无状态调用 |
| VLLLM | `h.providers.vlllm.ResponseWithImage()` | 图片理解，可降级到普通 LLM |
| MCP | `h.mcpManager.ExecuteTool(ctx, toolName, args)` | 工具调用 |

### 对话历史管理 (`connection.go:759-765`)
```go
// 用户消息添加到历史
h.dialogueManager.Put(chat.Message{Role: "user", Content: text})

// 每次 LLM 调用传递完整历史
h.genResponseByLLM(ctx, h.dialogueManager.GetLLMDialogue(), round)
```

**关键点**：对话历史存在 `ConnectionHandler.dialogueManager` 中，每次调用 LLM 都传完整消息列表，Provider 本身无状态。

## 日志系统

| 文件 | 说明 |
|------|------|
| `src/core/utils/logger.go` | 自定义彩色日志输出 |
| `src/configs/config.go` | 日志配置结构 |
| `src/main.go` | 日志初始化 |

### 当前日志格式
```
[2026-01-10 12:00:00.000] [INFO] [source:行号] 消息内容 {key=value}
```

**实际输出示例**：
```
[2026-01-11 10:30:45.123] [INFO] [transport.go:128] [WebSocket] [连接建立 abc123] 资源已分配 {device=dev1}
```

> 日志格式包含 `[source:行号]` 组件，用于定位日志输出位置。

### 专用日志方法
```go
logger.InfoASR(msg)    // [ASR] 前缀，品红色
logger.InfoLLM(msg)    // [LLM] 前缀，蓝色
logger.InfoTTS(msg)    // [TTS] 前缀，亮品红色
logger.InfoTiming(msg) // [TIMING] 前缀，亮绿色
```

## 文件结构

```
src/
├── main.go                    # 入口
├── core/
│   ├── utils/
│   │   └── logger.go         # 日志系统
│   ├── transport/
│   │   └── websocket/
│   │       └── transport.go  # WebSocket 入口
│   ├── connection.go         # 连接处理核心
│   ├── pool/
│   │   └── manager.go        # Provider 池
│   ├── providers/
│   │   ├── asr/
│   │   ├── llm/
│   │   └── tts/
│   └── dialogue/
│       └── manager.go        # 对话历史管理
└── configs/
    ├── config.go             # 配置结构
    └── config_default_init.go # 默认配置
```
