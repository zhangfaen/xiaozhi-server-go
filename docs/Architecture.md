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
main.go → StartTransportServer()
  → WebSocketTransport.Start()
  → handleWebSocket()
  → ConnectionContextAdapter.Handle()
  → ConnectionHandler.Handle()
```

### ConnectionHandler 结构 (`connection.go:90-94`)
```go
type ConnectionHandler struct {
    sessionID     string  // 服务端会话ID
    deviceID      string  // 设备ID
    clientId      string  // 客户端ID
    transportType string  // 传输类型
    dialogueManager *dialogue.Manager  // 对话历史管理
    providers *pool.ProviderSet  // Provider 池（复用）
}
```

## 消息处理 (`connection.go:77-100`)

| 消息类型 | 处理函数 | 说明 |
|----------|----------|------|
| `hello` | `handleHello()` | 初始化音频参数 |
| `chat` | `handleChat()` | 文本聊天 |
| `listen` | `handleListen()` | 语音识别控制 |
| `image` | `handleImage()` | 图片消息 |
| `mcp` | `handleMCP()` | MCP 工具调用 |
| `abort` | `handleAbort()` | 中断当前对话 |

## Provider 调用方式

**Provider 是池化的、无状态的**，每个连接从池中获取独立的 ProviderSet。

| Provider | 调用方式 | 关联方式 |
|----------|----------|----------|
| LLM | `h.providers.llm.ResponseWithFunctions(ctx, sessionID, messages, tools)` | 通过 sessionID 关联 |
| ASR | `h.providers.asr.AddAudio()` / `SetListener(h)` | 回调绑定到 ConnectionHandler |
| TTS | `h.providers.tts.ToTTS(text)` | 无状态调用 |

### 对话历史管理 (`connection.go:760-765`)
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
[2026-01-10 12:00:00.000] [INFO] 消息内容 {key=value}
```

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
