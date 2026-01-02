// +build ignore

// 测试小智服务器的 Doubao TTS 配置
package main

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

const (
	appid       = "8833371206"
	accessToken = "-1yBtIJ3p6l3ApQC6zZhhH_ZEkaPV5o-"
	cluster     = "volcano_tts"
	voice       = "zh_female_vv_uranus_bigtts"
)

var defaultHeader = []byte{0x11, 0x10, 0x11, 0x00}

func main() {
	fmt.Println("=== 测试火山引擎 TTS (WebSocket) ===")
	fmt.Println()

	testDoubaoTTS()
}

func testDoubaoTTS() {
	fmt.Println("连接到火山引擎 TTS WebSocket...")

	url := "wss://openspeech.bytedance.com/api/v1/tts/ws_binary"
	header := http.Header{
		"Authorization": []string{fmt.Sprintf("Bearer;%s", accessToken)},
	}

	conn, _, err := websocket.DefaultDialer.Dial(url, header)
	if err != nil {
		fmt.Printf("连接失败: %v\n", err)
		return
	}
	defer conn.Close()

	fmt.Println("连接成功!")

	// 准备请求参数
	reqParams := map[string]map[string]interface{}{
		"app": {
			"appid":   appid,
			"token":   accessToken,
			"cluster": cluster,
		},
		"user": {
			"uid": "test-user-001",
		},
		"audio": {
			"voice_type":   voice,
			"encoding":     "mp3",
			"speed_ratio":  1.0,
			"volume_ratio": 1.0,
			"pitch_ratio":  1.0,
		},
		"request": {
			"reqid":     fmt.Sprintf("test-%d", time.Now().UnixNano()),
			"text":      "你好，我是小智，这是语音合成测试",
			"text_type": "plain",
			"operation": "submit",
		},
	}

	// 序列化并压缩
	jsonData, _ := json.Marshal(reqParams)
	fmt.Printf("请求: %s\n", string(jsonData))

	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	w.Write(jsonData)
	w.Close()
	compressed := b.Bytes()

	// 构建二进制请求
	payloadSize := make([]byte, 4)
	binary.BigEndian.PutUint32(payloadSize, uint32(len(compressed)))
	request := make([]byte, len(defaultHeader))
	copy(request, defaultHeader)
	request = append(request, payloadSize...)
	request = append(request, compressed...)

	// 发送
	fmt.Println("发送请求...")
	if err := conn.WriteMessage(websocket.BinaryMessage, request); err != nil {
		fmt.Printf("发送失败: %v\n", err)
		return
	}

	// 接收音频
	fmt.Println("接收响应...")
	var audioData []byte
	messageCount := 0

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			fmt.Printf("接收失败: %v\n", err)
			break
		}

		messageCount++

		// 解析响应
		if len(message) < 4 {
			fmt.Printf("消息 %d: 数据太短\n", messageCount)
			continue
		}

		messageType := message[1] >> 4
		headSize := message[0] & 0x0f
		payload := message[headSize*4:]

		switch messageType {
		case 0xb: // audio
			seqNum := int32(binary.BigEndian.Uint32(payload[0:4]))
			audioLen := binary.BigEndian.Uint32(payload[4:8])
			audioData = append(audioData, payload[8:8+audioLen]...)
			fmt.Printf("消息 %d: 音频数据 seq=%d len=%d\n", messageCount, seqNum, audioLen)
			if seqNum < 0 {
				fmt.Println("收到最后一条音频数据")
			}
		case 0xf: // error
			code := int32(binary.BigEndian.Uint32(payload[0:4]))
			errMsg := payload[8:]
			// 尝试解压
			r, _ := gzip.NewReader(bytes.NewReader(errMsg))
			if r != nil {
				if d, _ := io.ReadAll(r); len(d) > 0 {
					errMsg = d
				}
				r.Close()
			}
			fmt.Printf("错误: [%d] %s\n", code, string(errMsg))
			return
		default:
			fmt.Printf("消息 %d: 类型=%d\n", messageCount, messageType)
		}

		if len(audioData) > 0 {
			// 检查是否最后一条
			if len(payload) >= 8 {
				seqNum := int32(binary.BigEndian.Uint32(payload[0:4]))
				if seqNum < 0 {
					break
				}
			}
		}
	}

	fmt.Printf("\n总共接收 %d 条消息\n", messageCount)
	fmt.Printf("音频数据大小: %d bytes\n", len(audioData))

	if len(audioData) > 0 {
		// 检查文件头
		fmt.Printf("文件头: %x\n", audioData[:12])

		// 保存音频
		outputFile := "test_output.mp3"
		if err := os.WriteFile(outputFile, audioData, 0644); err != nil {
			fmt.Printf("保存失败: %v\n", err)
			return
		}
		fmt.Printf("音频已保存到 %s\n", outputFile)
		fmt.Println("可以使用播放器播放: afplay test_output.mp3")
	} else {
		fmt.Println("没有收到音频数据")
	}
}
