# 小智服务器 (xiaozhi-server-go) 配置说明

## 当前状态

| 服务 | 状态 | 说明 |
|------|------|------|
| **TTS** | ✅ 正常 | DoubaoTTS (zh_female_vv_uranus_bigtts) |
| **LLM** | ✅ 正常 | OllamaLLM (qwen3:latest, 5.2GB) |
| **ASR** | ✅ 正常 | DoubaoASR - 协议正常工作，真实语音识别成功 |
| **服务器** | ✅ 运行中 | ws://0.0.0.0:8000, http://localhost:8080 |

## 测试结果

### ASR 测试 (2026-01-02)
```
发送音频: header=[17 32 0 0], size=4, data=64000
音频已发送
结束帧已发送（空音频，isLast=true）

--- 收到消息 ---
Header: version=1, headerSize=1, messageType=0x9, flags=0x1, serialization=0x1, compression=0x0
  Sequence: 2
Full Response:
{"audio_info":{"duration":2000},"result":{"text":""}}
```
- 协议正常工作
- 音频 duration=2000ms (2秒) 正确识别
- text为空因为测试音频是纯音调(440Hz)，不是实际语音

### TTS → ASR 端到端测试 (2026-01-02)

**输入文本**: "你好，我是小智，这是语音合成测试"

**流程**:
1. TTS 生成音频: `go run smoke_test/test_doubao.go` → `test_output.mp3` (77KB)
2. 格式转换: MP3 → WAV → PCM 16kHz
3. ASR 识别: `go run smoke_test/test_asr.go smoke_test/test_tts_to_asr.pcm`

**识别结果**:
```json
{"audio_info":{"duration":3864},"result":{"text":"你好，我是小智。"}}
{"audio_info":{"duration":3864},"result":{"text":"这是语音合成测试。"}}
```

**结论**: ✅ ASR 完美识别 TTS 生成的语音！

## 测试文件目录

```
smoke_test/
├── test_doubao.go       # TTS 测试脚本
├── test_asr.go          # ASR 测试脚本
├── test_asr.sh          # ASR shell 测试
├── create_pcm_audio.py  # 生成 PCM 测试音频
├── test_output.mp3      # TTS 生成的语音
├── test_input.pcm       # 440Hz 正弦波 (2秒)
├── test_tts_to_asr.pcm  # TTS→ASR 测试用的 PCM
└── test_tts_to_asr.wav  # 中间 WAV 文件
```

## 运行测试

```bash
# 测试 TTS
cd smoke_test && go run test_doubao.go

# TTS 输出转 PCM 16kHz (Mac)
afconvert -f WAVE -d LEI16@16000 -c 1 test_output.mp3 test.wav
sox test.wav -r 16000 -c 1 -b 16 -e signed-integer -t raw test_tts_to_asr.pcm

# 测试 ASR
go run test_asr.go test_tts_to_asr.pcm
```

## 重要发现

- **ASR 不接受 gzip 压缩** - 需要发送原始 PCM 数据
- **音频格式** - PCM 16kHz, 16bit, 单声道
- **响应分段** - 长音频会被分段返回，每段带 sequence 号

## 火山引擎凭据

- **AppID**: 8833371206
- **Access Token**: -1yBtIJ3p6l3ApQC6zZhhH_ZEkaPV5o-
- **Secret Key**: C-Tz1tzPxVyqMXoA8XVcUUxVWmq6EUuq
- **TTS Cluster**: volcano_tts
- **TTS Voice**: zh_female_vv_uranus_bigtts

## 目录结构

```
~/faen/dev/xiaozhi-server-go/
├── config.yaml          # 主配置文件
├── config.db            # SQLite 数据库（包含模块配置）
├── .env                 # 环境变量 (CGO_ENABLED=1)
├── run_on_mac.sh        # 启动脚本
├── test_doubao.go       # TTS 测试脚本
├── test_asr.go          # ASR 测试脚本
├── logs/
│   └── server.log       # 服务器日志
└── tmp/                 # 临时文件目录
```

## 启动命令

```bash
cd ~/faen/dev/xiaozhi-server-go
./run_on_mac.sh
```

## Ollama 配置

```bash
# Ollama 已安装并运行
ollama serve           # 后台运行
ollama pull qwen3      # 已下载 (5.2GB)

# 测试 Ollama
curl http://localhost:11434/api/tags
```

## 待解决问题

1. **ASR 协议兼容性问题**
   - 火山引擎 ASR 返回错误: "unsupported protocol version"
   - 小智服务器的 Doubao ASR 实现可能需要更新以适配最新的火山引擎 API
   - 需要用实际 ESP32 设备测试完整流程

2. **音频格式要求**
   - ASR 需要 PCM 16kHz 格式
   - TTS 输出的是 MP3 24kHz
   - 小智服务器内部有音频格式转换逻辑

## 服务器日志查看

```bash
tail -f logs/server.log
```

## 测试脚本

```bash
# 测试 TTS
go run test_doubao.go

# 测试 ASR（需要 PCM 格式音频）
go run test_asr.go
```

## ESP32 设备连接配置

- WebSocket URL: `ws://192.168.0.104:8000`
- Token: config.yaml 中配置的 server.token

## 下一步工作

1. 用 ESP32 设备连接服务器测试完整流程（ASR → LLM → TTS）
2. 如果 ASR 失败，检查火山引擎控制台确认 ASR 服务权限
3. 可能需要更新小智服务器的 Doubao ASR 实现以适配最新 API
