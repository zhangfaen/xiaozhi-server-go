#!/usr/bin/env python3
"""
生成虚拟客户端对话音频
使用豆包 TTS WebSocket API 生成 PCM 16kHz 音频
"""

import asyncio
import json
import os
import struct
import gzip
from pathlib import Path

import websockets

APPID = "8833371206"
ACCESS_TOKEN = "-1yBtIJ3p6l3ApQC6zZhhH_ZEkaPV5o-"
CLUSTER = "volcano_tts"
VOICE = "zh_female_vv_uranus_bigtts"
TTS_URL = "wss://openspeech.bytedance.com/api/v1/tts/ws_binary"

# 对话内容
CONVERSATIONS = [
    ("user_hello_weather", "小智你好，我想问问北京今天天气怎么样"),
    ("ai_weather_response", "今天北京天气晴朗，温度25到18度，适合户外活动。"),
    ("user_tourist", "那北京有什么好玩的地方推荐吗"),
    ("ai_tourist_response", "北京有很多好玩的地方，故宫、长城、颐和园、天坛都是必去的景点。您还可以去南锣鼓巷感受老北京风情。"),
    ("user_thanks", "好的，谢谢小智，再见"),
    ("ai_goodbye_response", "不客气，祝您在北京玩得开心，再见！"),
]

async def generate_tts(text: str, output_path: str):
    """调用豆包 TTS 生成音频"""
    print(f"生成音频: {output_path}")
    print(f"  文本: {text}")

    header = bytes([0x11, 0x10, 0x11, 0x00])

    req_params = {
        "app": {
            "appid": APPID,
            "token": ACCESS_TOKEN,
            "cluster": CLUSTER,
        },
        "user": {"uid": "virtual-client"},
        "audio": {
            "voice_type": VOICE,
            "encoding": "mp3",
            "speed_ratio": 1.0,
            "volume_ratio": 1.0,
            "pitch_ratio": 1.0,
        },
        "request": {
            "reqid": f"tts-{output_path}",
            "text": text,
            "text_type": "plain",
            "operation": "submit",
        },
    }

    json_data = json.dumps(req_params)
    compressed = gzip.compress(json_data.encode())

    payload_size = struct.pack('>I', len(compressed))
    request = header + payload_size + compressed

    async with websockets.connect(TTS_URL) as ws:
        await ws.send(request)
        print(f"  请求已发送")

        audio_data = bytearray()
        message_count = 0

        while True:
            message = await ws.recv()
            message_count += 1

            if len(message) < 4:
                continue

            message_type = message[1] >> 4
            head_size = message[0] & 0x0f
            payload = message[head_size * 4:]

            if message_type == 0xb:  # audio
                seq_num = int.from_bytes(payload[0:4], 'big')
                audio_len = int.from_bytes(payload[4:8], 'big')
                audio_data.extend(payload[8:8 + audio_len])
                if seq_num < 0:
                    break

            elif message_type == 0xf:  # error
                code = int.from_bytes(payload[0:4], 'big')
                err_msg = payload[8:].decode('utf-8', errors='ignore')
                print(f"  错误: [{code}] {err_msg}")
                return False

    if audio_data:
        # 检查是否是 MP3 文件头
        if audio_data[:3] == b'\xff\xfb' or audio_data[:3] == b'\xff\xfa':
            # 保存 MP3
            mp3_path = output_path.replace('.pcm', '.mp3')
            with open(mp3_path, 'wb') as f:
                f.write(audio_data)
            print(f"  保存 MP3: {mp3_path} ({len(audio_data)} bytes)")

            # 转换为 PCM 16kHz
            pcm_path = output_path
            cmd = [
                'afconvert', '-f', 'WAVE', '-d', 'LEI16@16000', '-c', '1',
                mp3_path, mp3_path.replace('.pcm', '.wav')
            ]
            subprocess.run(cmd, capture_output=True)

            cmd = [
                'sox',
                mp3_path.replace('.pcm', '.wav'),
                '-r', '16000', '-c', '1', '-b', '16',
                '-e', 'signed-integer', '-t', 'raw', pcm_path
            ]
            subprocess.run(cmd, capture_output=True)

            os.remove(mp3_path.replace('.pcm', '.wav'))
            os.remove(mp3_path)

            print(f"  保存 PCM: {pcm_path} ({len(audio_data)} bytes)")
            return True
        else:
            # 直接保存为 PCM (原始音频数据)
            with open(output_path, 'wb') as f:
                f.write(audio_data)
            print(f"  保存 PCM: {output_path} ({len(audio_data)} bytes)")
            return True
    else:
        print("  未收到音频数据")
        return False

async def main():
    print("=" * 60)
    print("虚拟客户端对话音频生成")
    print("=" * 60)

    audio_dir = Path("audio")
    audio_dir.mkdir(exist_ok=True)

    success_count = 0
    for filename, text in CONVERSATIONS:
        output_path = str(audio_dir / f"{filename}.pcm")
        if await generate_tts(text, output_path):
            success_count += 1
        print()

    print(f"成功生成 {success_count}/{len(CONVERSATIONS)} 个音频文件")
    print("\n生成的音频文件:")
    for filename, _ in CONVERSATIONS:
        pcm_path = audio_dir / f"{filename}.pcm"
        if pcm_path.exists():
            size = pcm_path.stat().st_size
            duration = size / 32000  # 16kHz * 2 bytes per sample
            print(f"  {filename}.pcm - {size} bytes - {duration:.1f}s")

if __name__ == "__main__":
    asyncio.run(main())
