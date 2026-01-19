# WebSocket 客户端音频 Demo

## 概述

这是一个基于 WebSocket 的实时语音交互客户端 Demo，实现了以下功能：

- **自动语音检测（VAD）**：检测到说话自动开始录音
- **自动提交**：连续 1000ms 静音自动提交音频
- **TTS 暂停/恢复**：TTS 播放时自动暂停录音，防止回声
- **Opus 编码**：使用 Opus 编码传输音频数据
- **录音保存**：可选保存录音为 WAV 文件

## 编译运行

```bash
# 编译
go build ./demo/websocket_client/

# 运行
./websocket_client
```

## 文件结构

```
demo/websocket_client/
├── websocket_client.go    # WebSocket 客户端实现
├── audio_recorder.go      # 音频录制器实现
└── recorder_files/        # 录音文件保存目录
```

## 核心组件

### AudioRecorder

音频录制器，负责麦克风采集、VAD 检测和音频编码。

**主要功能：**

- `Start()` - 启动自动录音服务
- `Pause()` - 暂停录音（TTS 播放时调用）
- `Resume()` - 恢复录音（TTS 播放结束时调用）
- `Close()` - 关闭录制器
- `Shutdown()` - 完全关闭（终止 PortAudio）
- `WaitForExit()` - 等待用户按 Ctrl+C 退出

**状态机：**

```
StateIdle → StateRecording → StateSubmitting → StateWaiting → StateIdle
                                      ↑
                    (TTS播放时)--------+
                                      |
                               StatePaused
```

**VAD 参数：**

VAD（Voice Activity Detection）用于检测用户是否在说话，通过计算音频帧的能量来判断。

| 参数 | 默认值 | 说明 | 经验值建议 |
|------|--------|------|-----------|
| vadThreshold | 500 | 语音能量阈值（0-32767），能量超过此值认为有语音 | 环境安静用 300-500，环境嘈杂用 800-1000 |
| silenceTimeout | 1000ms | 连续静音超时时间，超时后自动提交录音 | 对话场景 800-1500ms，唤醒词场景 300-500ms |
| minRecordDuration | 500ms | 最小录音时长，防止误触发后立即停止 | 保持 300-500ms |
| maxRecordDuration | 30s | 最大录音时长，超时强制提交 | 保持 20-30s，避免录音过长 |

**参数调整建议：**

1. **vadThreshold（阈值）**
   - 阈值越低越灵敏，容易检测到轻声说话
   - 阈值越高越保守，需要较大声才能触发
   - 建议：先在目标环境测试，取环境底噪的 2-3 倍值

2. **silenceTimeout（静音超时）**
   - 时间越长，用户停顿不会被中断
   - 时间越短，响应更快但容易打断自然对话
   - 建议：对话场景用 1000ms，唤醒词场景用 300-500ms

3. **minRecordDuration（最小录音时长）**
   - 防止 VAD 误触发导致的频繁短录音
   - 建议：保持 300-500ms 即可

4. **maxRecordDuration（最大录音时长）**
   - 防止用户一直说话导致录音过长
   - 建议：20-30s 足够一次完整表达

### WebSocketClient

WebSocket 客户端，负责与服务器通信。

**主要功能：**

- `Connect()` - 连接到 WebSocket 服务器
- `SendTextMessage()` - 发送文本消息
- `SendTextMessageToServer()` - 发送控制消息（如 listen）
- `SendAudioMessage()` - 发送音频数据
- `StartListening()` - 开始监听服务器消息
- `Close()` - 关闭客户端

**消息处理：**

- 文本消息：解析 JSON，处理 TTS 暂停/恢复
- 二进制消息：解码 Opus 音频并播放

## 核心工作流程

### 1. WebSocket 连接建立

```
创建 WebSocketClient → Connect(url, deviceID, clientID)
                                        ↓
                              发送 Hello 消息
                              {
                                "type": "hello",
                                "audio_params": {
                                  "format": "opus",
                                  "sample_rate": 24000,
                                  "channels": 1,
                                  "frame_duration": 60
                                }
                              }
                                        ↓
                              服务器返回 Welcome
                                        ↓
                              初始化扬声器，启动播放
                                        ↓
                              启动消息监听 goroutine
```

### 2. 接收并播放服务器音频

```
服务器发送 Opus 音频数据 (BinaryMessage)
                              ←
                    WebSocketClient.StartListening()
                              ↓
              handleBinaryMessage() 解码 Opus
                              ↓
              DynamicStreamer.AddData()
                              ↓
              speaker.Play() 播放音频
```

### 3. 录音流程（完全基于本地 VAD 检测）

```
用户开始说话 → VAD检测(能量>500) → StateIdle → StateRecording
                                           ↓
                              发送 listen.start → 累积音频数据
                                           ↓
                              Opus编码 → 发送到服务器
                                           ↓
                              用户停止说话 → 连续静音1000ms
                                           ↓
                              发送 listen.stop → StateSubmitting
                                           ↓
                              保存录音文件(可选) → StateWaiting
                                           ↓
                              500ms后 → StateIdle
```

**说明：**
- 录音的**开始和结束完全由本地 VAD（Voice Activity Detection）控制**
- 不依赖服务器的 TTS 状态消息
- 检测到语音能量 > 500 开始录音
- 连续静音 1000ms 自动停止录音

### 消息时序图

```
客户端                                              服务器
  |                                                   |
  |  创建 WebSocketClient                             |
  |                                                   |
  |----------- Hello (audio_params) ----------------->|
  |                                                   |
  |                    <---------- Welcome -----------|
  |                                                   |
  |  初始化扬声器，启动播放                           |
  |                                                   |
  |  启动消息监听线程                                 |
  |                                                   |
  |----------- listen.start ------------------------->|
  |----------- Opus Audio --------------------------->|
  |----------- Opus Audio --------------------------->|
  |                      ...                          |
  |----------- listen.stop -------------------------->|
  |                                                   |
  |                    <----------- Opus Audio ------|  (TTS)
  |                                                   |
  |  解码 Opus → 播放                                 |
  |                                                   |
```

## 通信协议

### 客户端 -> 服务器

**Hello 消息（连接时发送）：**

```json
{
  "type": "hello",
  "audio_params": {
    "format": "opus",
    "sample_rate": 24000,
    "channels": 1,
    "frame_duration": 60
  }
}
```

**Listen 消息（控制录音）：**

```json
{
  "type": "listen",
  "state": "start" | "stop",
  "mode": "realtime"
}
```

**Chat 消息（发送文本）：**

```json
{
  "type": "chat",
  "data": "文本内容",
  "audio_format": "pcm",
  "sample_rate": 24000
}
```

**音频数据：**

- 格式：Opus 编码的二进制数据
- 采样率：24000 Hz
- 声道数：1（单声道）
- 帧长：60ms

### 服务器 -> 客户端

**TTS 消息（控制暂停/恢复）：**

```json
{
  "type": "tts",
  "state": "start" | "stop"
}
```

**音频数据：**

- 格式：Opus 编码的二进制数据
- 解码后播放

## 配置参数

在 `main()` 函数中可配置：

```go
url := "ws://localhost:8000"          // 服务器地址
deviceID := "virtual-esp32s3"         // 设备 ID
clientID := "virtual-esp32s3-001"     // 客户端 ID
```

## 依赖

- `github.com/gordonklaus/portaudio` - 音频采集
- `github.com/qrtc/opus-go` - Opus 编解码
- `github.com/gorilla/websocket` - WebSocket 客户端
- `github.com/faiface/beep` - 音频播放

## 注意事项

1. **麦克风权限**：需要授予程序麦克风访问权限，macos 上需要在系统设置 -> 隐私与安全性 -> 麦克风 中授权
2. **PortAudio**：首次运行会列出可用的麦克风设备供选择
3. **TTS 冲突**：TTS 播放时会自动暂停录音，防止回声
4. **录音保存**：设置 `shouldSaveFile` 为 `true` 可保存录音文件
