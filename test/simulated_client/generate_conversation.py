#!/usr/bin/env python3
"""
生成虚拟客户端对话音频
使用豆包 TTS 生成 PCM 16kHz 音频
"""

import os
import subprocess
import json

# 对话内容
CONVERSATIONS = [
    {
        "filename": "audio/user_hello_weather.pcm",
        "text": "小智你好，我想问问北京今天天气怎么样",
        "speaker": "user"
    },
    {
        "filename": "ai_weather_response.pcm",
        "text": "今天北京天气晴朗，温度25到18度，适合户外活动。",
        "speaker": "ai"
    },
    {
        "filename": "audio/user_tourist.pcm",
        "text": "那北京有什么好玩的地方推荐吗",
        "speaker": "user"
    },
    {
        "filename": "ai_tourist_response.pcm",
        "text": "北京有很多好玩的地方，故宫、长城、颐和园、天坛都是必去的景点。您还可以去南锣鼓巷感受老北京风情。",
        "speaker": "ai"
    },
    {
        "filename": "audio/user_thanks.pcm",
        "text": "好的，谢谢小智，再见",
        "speaker": "user"
    },
    {
        "filename": "ai_goodbye_response.pcm",
        "text": "不客气，祝您在北京玩得开心，再见！",
        "speaker": "ai"
    }
]

def generate_tts(text, output_file):
    """调用豆包 TTS 生成音频"""
    print(f"\n生成: {output_file}")
    print(f"  文本: {text}")

    # 调用 TTS 测试脚本
    cmd = [
        "go", "run", "smoke_test/test_doubao.go"
    ]

    # 由于 test_doubao.go 是固定的，我们直接运行并手动修改
    # 这里我们用另一种方式：直接调用 TTS WebSocket API
    print("  (需要手动生成，调用 TTS 服务...)")

    return True

if __name__ == "__main__":
    print("=" * 60)
    print("虚拟客户端对话音频生成")
    print("=" * 60)

    # 确保 audio 目录存在
    os.makedirs("audio", exist_ok=True)

    print("\n请手动运行以下命令生成对话音频:")
    print("-" * 60)
    for conv in CONVERSATIONS:
        if conv["speaker"] == "user":
            print(f"# 用户: {conv['text']}")
        else:
            print(f"# AI: {conv['text']}")
    print("-" * 60)
    print("\n提示: 需要修改 smoke_test/test_doubao.go 中的文本，然后运行")
