# 小智服务器 (xiaozhi-server-go) 配置说明

## 当前状态

| 服务 | 状态 | 说明 |
|------|------|------|
| **TTS** | ✅ 正常 | DoubaoTTS (zh_female_vv_uranus_bigtts) |
| **LLM** | ✅ 正常 | OllamaLLM (qwen3:latest, 5.2GB) |
| **ASR** | ⚠️ 待验证 | DoubaoASR - 需要实际设备测试 |
| **服务器** | ✅ 运行中 | ws://0.0.0.0:8000, http://localhost:8080 |

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
