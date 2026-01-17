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
// 如果请求头没有提供，尝试从 URL query 参数获取
// device-id, client-id
// 如果仍然没有，clientID 使用 conn 的内存地址作为 fallback
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

> **注意**: `ConnectionContextAdapter` 是连接适配层，负责将 `WebSocketConnection` 转换为统一的 `Connection` 接口。

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

### WebSocket 消息帧类型

| messageType | 方向 | 内容格式 | 用途 |
|-------------|------|----------|------|
| **1** | 双向 | JSON 文本 | 指令消息、控制信令 |
| **2** | 双向 | 二进制数据 | 音频数据流（PCM/Opus） |

**关键设计**：利用 WebSocket 协议帧头自带的 messageType 区分数据类型，而非在 payload 中额外标记。

### 客户端 → 服务端 文本指令 (`type` 字段)

| type | 处理函数 | 用途 |
|------|----------|------|
| `hello` | `handleHelloMessage()` | 建立连接时上报客户端音频参数（format/sample_rate/channels/frame_duration） |
| `chat` | `handleChatMessage()` | 文本聊天 |
| `listen` | `handleListenMessage()` | 语音识别控制（start/stop/detect） |
| `abort` | `clientAbortChat()` | 打断当前对话/语音 |
| `image` | `handleImageMessage()` | 图片消息（携带 url 或 base64 数据） |
| `vision` | `handleVisionMessage()` | 视觉能力控制（gen_pic/gen_video/read_img） |
| `iot` | `handleIotMessage()` | 物联网设备状态同步（descriptors/states） |
| `mcp` | `mcpManager.HandleXiaoZhiMCPMessage()` | MCP 协议工具调用 |

### listen 子指令 (`state` 字段)

| state | 用途 |
|-------|------|
| `start` | 开始拾音（用户开始说话，可打断当前 LLM 生成） |
| `stop` | 停止拾音（发送空数据标记 ASR 识别结束） |
| `detect` | 检测消息（可携带 `text` 参数） |

#### listen `mode` 参数（**重要**）

客户端可以通过 `mode` 参数指定三种不同的拾音模式：

| mode | 触发时机 | 是否打断语音 | 文本累积 | 典型场景 |
|------|----------|-------------|----------|----------|
| `auto` | ASR 返回任意结果 | 不打断 | 不累积 | 智能音箱 |
| `manual` | 用户主动 stop 或 ASR 返回最终结果 | 不打断 | **累积** | 按键录音设备 |
| `realtime` | ASR 返回任意结果 | **立即打断** | 不累积 | 可打断的对话机器人 |

**三种模式的行为示例**：

```json
// auto 模式：用户说"今天"，AI 就开始回答
{"type":"listen", "state":"start", "mode":"auto"}

// manual 模式：用户说完一整句后，AI 才回答
{"type":"listen", "state":"start", "mode":"manual"}
// 用户按住录音键说话...
{"type":"listen", "state":"stop"}

// realtime 模式：用户随时可以打断 AI 说话
{"type":"listen", "state":"start", "mode":"realtime"}
// 如果用户中途说话，立即打断当前播放并处理新内容
```

> **注意**: `mode` 字段默认为 `auto`。

### vision 子指令 (`cmd` 字段)

| cmd | 用途 |
|-----|------|
| `gen_pic` | 生成图片 |
| `gen_video` | 生成视频 |
| `read_img` | 读取/分析图片 |

### MCP 消息 (`type: mcp`)

MCP（Model Context Protocol）消息用于工具调用，封装了 JSON-RPC 2.0 协议。

**消息结构**：

```json
{
  "type": "mcp",
  "session_id": "abc123",
  "payload": {
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize | tools/list | tools/call",
    "params": { ... }
  }
}
```

#### 常见 method

| method | 用途 | 方向 |
|--------|------|------|
| `initialize` | MCP 初始化协商 | 客户端 → 服务端 |
| `tools/list` | 获取可用工具列表 | 客户端 → 服务端 |
| `tools/call` | 调用工具 | 客户端 → 服务端 |
| `notifications/*` | 通知事件 | 双向 |

#### tools/call 请求示例

```json
{
  "type": "mcp",
  "session_id": "abc123",
  "payload": {
    "jsonrpc": "2.0",
    "id": 3,
    "method": "tools/call",
    "params": {
      "name": "get_weather",
      "arguments": {"city": "北京"}
    }
  }
}
```

#### 响应示例

```json
{
  "type": "mcp",
  "session_id": "abc123",
  "payload": {
    "jsonrpc": "2.0",
    "id": 3,
    "result": {
      "content": [
        {
          "type": "text",
          "text": "北京今天天气晴朗，25度"
        }
      ],
      "isError": false
    }
  }
}
```

### 服务端 → 客户端 文本消息 (`type` 字段)

| type | 用途 |
|------|------|
| `hello` | 响应客户端，携带 `session_id` 和服务端音频参数 |
| `stt` | ASR 语音识别结果 |
| `tts` | TTS 状态通知（`start`/`sentence_start`/`sentence_end`/`stop`） |
| `llm` | LLM 响应文本（带 `emotion` 情绪标签） |

#### `hello` 响应消息

```json
{
  "type": "hello",
  "version": 1,
  "transport": "websocket",
  "session_id": "abc123",
  "audio_params": {
    "format": "opus",
    "sample_rate": 24000,
    "channels": 1,
    "frame_duration": 60,
    "audio_codec": "opus"
  }
}
```

> **关键字段**：`audio_codec` 表示服务端返回音频使用的编码格式（`opus` 或 `pcm`），客户端需据此解码。

#### `tts` 状态消息（完整状态流）

| state | 触发时机 | 说明 |
|-------|----------|------|
| `start` | 开始 TTS 合成 | 服务端开始生成语音 |
| `sentence_start` | 开始发送音频帧 | 即将发送音频数据（type=2） |
| `sentence_end` | 结束发送音频帧 | 当前句子音频发送完成 |
| `stop` | 全部完成 | 所有音频发送完毕，对话结束 |

```json
// 开始合成
{"type": "tts", "state": "start", "session_id": "abc123", "text": "", "index": 0, "audio_codec": "opus"}

// 开始发送音频帧
{"type": "tts", "state": "sentence_start", "session_id": "abc123", "text": "今天天气晴朗", "index": 1, "audio_codec": "opus"}

// ... 客户端接收音频帧 (type=2) ...

// 结束发送音频帧
{"type": "tts", "state": "sentence_end", "session_id": "abc123", "text": "今天天气晴朗", "index": 1, "audio_codec": "opus"}

// 全部完成
{"type": "tts", "state": "stop", "session_id": "abc123", "text": "", "index": 1, "audio_codec": "opus"}
```

#### `llm` 情绪消息

```json
{"type": "llm", "text": "👀", "emotion": "thinking", "session_id": "abc123"}
```

**支持的 `emotion` 枚举值**：

| emotion | 表情 | 含义 |
|---------|------|------|
| `neutral` | 😐 | 中性 |
| `happy` | 😊 | 开心 |
| `laughing` | 😂 | 大笑 |
| `funny` | 🤡 | 有趣 |
| `sad` | 😢 | 悲伤 |
| `angry` | 😠 | 生气 |
| `crying` | 😭 | 哭泣 |
| `loving` | 🥰 | 喜爱 |
| `embarrassed` | 😳 | 尴尬 |
| `surprised` | 😮 | 惊讶 |
| `shocked` | 😱 | 震惊 |
| `thinking` | 🤔 | 思考中 |
| `winking` | 😉 | 眨眼 |
| `cool` | 😎 | 酷 |
| `relaxed` | 😌 | 放松 |
| `delicious` | 😋 | 美味 |
| `kissy` | 😘 | 飞吻 |
| `confident` | 😏 | 自信 |
| `sleepy` | 😴 | 困倦 |
| `silly` | 🤪 | 傻乎乎 |
| `confused` | 😕 | 困惑 |

### 消息处理流程

```
客户端 WebSocket 消息帧
        │
        ├── type=1 (TEXT=1) → clientTextQueue → processClientTextMessage()
        │                                            │
        │                                            └── 解析 JSON type 字段分发
        │                                                hello/chat/listen/abort/image/...
        │
        └── type=2 (BINARY=2) → clientAudioQueue → 音频处理管道
                                    │
                                    └── opusDecoder 解码 (如果是 opus 格式)
                                        → ASR Provider 语音识别
```

### 消息示例

```json
// 客户端 hello（必填）
{"type":"hello","audio_params":{"format":"opus","sample_rate":24000,"channels":1,"frame_duration":60}}

// 客户端 listen start
{"type":"listen","state":"start","mode":"auto"}

// 客户端 listen detect (纯文本检测)
{"type":"listen","state":"detect","text":"今天天气怎么样"}

// 客户端 image（使用 URL）
{"type":"image","text":"这张图片里有什么","image_data":{"url":"http://...","format":"jpeg"}}

// 客户端 image（使用 base64 数据）
{"type":"image","text":"这张图片里有什么","image_data":{"data":"base64编码数据...","format":"jpeg"}}

// 服务端 hello 响应
{"type":"hello","version":1,"transport":"websocket","session_id":"abc123","audio_params":{"format":"opus",...}}

// 服务端 stt
{"type":"stt","text":"今天天气怎么样","session_id":"abc123"}

// 服务端 tts 状态
{"type":"tts","state":"sentence_start","session_id":"abc123","text":"今天天气晴朗","index":1,"audio_codec":"opus"}

// 服务端 llm 响应
{"type":"llm","text":"👀","emotion":"thinking","session_id":"abc123"}
```

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
