# 虚拟 ESP32S3 客户端

用于测试小智服务器的虚拟设备客户端，模拟 ESP32 设备的对话流程。

## 目录结构

```
esp32s3_virtual_session/
├── virtual_client.go        # 虚拟客户端主程序
├── generate_tts_audio.py    # 生成对话音频
├── audio/                   # 对话音频文件
│   ├── user_hello_weather.pcm
│   ├── user_tourist.pcm
│   └── user_thanks.pcm
└── README.md
```

## 对话场景

```
用户: 小智你好，我想问问北京今天天气怎么样
AI:   今天北京天气晴朗，温度25到18度，适合户外活动。

用户: 那北京有什么好玩的地方推荐吗
AI:   北京有很多好玩的地方，故宫、长城、颐和园、天坛都是必去的景点。
      您还可以去南锣鼓巷感受老北京风情。

用户: 好的，谢谢小智，再见
AI:   不客气，祝您在北京玩得开心，再见！
```

## 使用方法

### 1. 生成对话音频

```bash
# 安装依赖
pip install websockets

# 生成音频
python3 generate_tts_audio.py
```

### 2. 运行虚拟客户端

```bash
# 确保服务器已启动
cd ~/faen/dev/xiaozhi-server-go
./run_on_mac.sh

# 新终端运行虚拟客户端
cd ~/faen/dev/xiaozhi-server-go/esp32s3_virtual_session
go run virtual_client.go -server localhost:8000
```

## 工作流程

```
1. 虚拟客户端连接服务器 WebSocket
2. 发送 PCM 音频（用户语音）
3. 服务器调用 ASR 识别语音
4. 服务器调用 LLM 生成回复
5. 服务器调用 TTS 生成语音
6. 虚拟客户端收到 TTS 音频并播放
7. 重复步骤 2-6 完成多轮对话
```

## 依赖

- Go 1.24+
- Python 3.8+
- websockets (Python)
- sox (音频转换, Mac: `brew install sox`)
