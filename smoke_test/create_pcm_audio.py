#!/usr/bin/env python3
# 创建简单的 PCM 16kHz 音频用于 ASR 测试

import wave
import struct
import math
import os

# 参数
sample_rate = 16000  # 16kHz
duration = 2.0       # 2秒
amplitude = 8000     # 振幅

# 生成正弦波测试信号 (440Hz - A4 note)
frequency = 440
t = [i / sample_rate for i in range(int(sample_rate * duration))]

print(f"创建 PCM 音频: {sample_rate}Hz, {duration}秒")

# 生成音频数据
audio_data = []
for time_val in t:
    # 简单的正弦波
    value = int(amplitude * math.sin(2 * math.pi * frequency * time_val))
    # 加上一些随机噪声模拟真实语音的基频
    noise = int(500 * math.sin(2 * math.pi * 100 * time_val))
    value += noise
    # 确保在 -32768 到 32767 范围内
    value = max(-32768, min(32767, value))
    audio_data.append(struct.pack('<h', value))

# 写入 WAV 文件
wav_file = 'test_input.pcm.wav'
with wave.open(wav_file, 'wb') as wf:
    wf.setnchannels(1)        # 单声道
    wf.setsampwidth(2)        # 16bit
    wf.setframerate(sample_rate)
    wf.writeframes(b''.join(audio_data))

print(f"WAV 文件已创建: {wav_file}")
print(f"文件大小: {os.path.getsize(wav_file)} bytes")

# 同时创建原始 PCM 文件
pcm_file = 'test_input.pcm'
with open(pcm_file, 'wb') as f:
    f.write(b''.join(audio_data))
print(f"PCM 文件已创建: {pcm_file}")
print(f"文件大小: {os.path.getsize(pcm_file)} bytes")
print(f"期望时长: {os.path.getsize(pcm_file) / (sample_rate * 2):.2f} 秒")
