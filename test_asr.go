// +build ignore

// 测试火山引擎 ASR
package main

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

// Protocol constants
const (
	clientFullRequest  = 0x1
	clientAudioRequest = 0x2
	noSequence         = 0x0
	negSequence        = 0x2
	jsonFormat         = 0x1
	gzipCompression    = 0x1
)

const (
	appid       = "8833371206"
	accessToken = "-1yBtIJ3p6l3ApQC6zZhhH_ZEkaPV5o-"
)

func main() {
	fmt.Println("=== 测试火山引擎 ASR ===")
	fmt.Println()

	// 读取音频文件
	audioFile := "test_output.mp3"
	if len(os.Args) > 1 {
		audioFile = os.Args[1]
	}

	audioData, err := os.ReadFile(audioFile)
	if err != nil {
		fmt.Printf("读取音频文件失败: %v\n", err)
		return
	}

	fmt.Printf("音频文件: %s (%d bytes)\n", audioFile, len(audioData))
	fmt.Printf("格式: %s\n", getAudioFormat(audioData))

	// 对于测试，我们尝试直接发送音频
	// 注意：火山 ASR 可能需要特定格式
	testASR(audioData)
}

func getAudioFormat(data []byte) string {
	if len(data) < 12 {
		return "unknown"
	}
	// 检查 ID3 标签
	if data[0] == 'I' && data[1] == 'D' && data[2] == '3' {
		return "MP3 (ID3)"
	}
	// 检查 MP3 frame sync
	if (data[0]&0xFF) == 0xFF && (data[1]&0xE0) == 0xE0 {
		return "MP3"
	}
	// 检查 WAV header
	if string(data[0:4]) == "RIFF" && string(data[8:12]) == "WAVE" {
		return "WAV"
	}
	return fmt.Sprintf("0x%02x%02x%02x%02x...", data[0], data[1], data[2], data[3])
}

func testASR(audioData []byte) {
	fmt.Println("\n连接到火山引擎 ASR WebSocket...")

	url := "wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_nostream"
	connectID := fmt.Sprintf("test-%d", time.Now().UnixNano())

	headers := map[string][]string{
		"X-Api-App-Key":     {appid},
		"X-Api-Access-Key":  {accessToken},
		"X-Api-Resource-Id": {"volc.bigasr.sauc.duration"},
		"X-Api-Connect-Id":  {connectID},
	}

	conn, _, err := websocket.DefaultDialer.Dial(url, headers)
	if err != nil {
		fmt.Printf("连接失败: %v\n", err)
		return
	}
	defer conn.Close()

	fmt.Println("连接成功!")

	// 构造请求
	reqParams := map[string]interface{}{
		"user": map[string]interface{}{
			"uid": connectID,
		},
		"audio": map[string]interface{}{
			"format":   "mp3", // 尝试使用 mp3 格式
			"rate":     24000, // TTS 输出的采样率
			"bits":     16,
			"channel":  1,
			"language": "zh-CN",
		},
		"request": map[string]interface{}{
			"model_name":       "bigmodel",
			"end_window_size":  300,
			"enable_punc":      true,
			"enable_itn":       true,
			"enable_ddc":       false,
			"result_type":      "single",
			"show_utterances":  false,
		},
	}

	reqBytes, _ := json.Marshal(reqParams)
	fmt.Printf("请求: %s\n", string(reqBytes))

	// 压缩请求
	var buf bytes.Buffer
	gzipWriter := gzip.NewWriter(&buf)
	gzipWriter.Write(reqBytes)
	gzipWriter.Close()

	// 构造协议头 - 与服务器相同
	header := make([]byte, 4)
	header[0] = (1 << 4) | 1                           // version 1 + header size 1
	header[1] = (clientFullRequest << 4) | noSequence  // message type + flags
	header[2] = (jsonFormat << 4) | gzipCompression    // serialization + compression
	header[3] = 0                                      // reserved
	size := make([]byte, 4)
	binary.BigEndian.PutUint32(size, uint32(buf.Len()))
	fullRequest := append(header, size...)
	fullRequest = append(fullRequest, buf.Bytes()...)

	// 发送请求
	if err := conn.WriteMessage(websocket.BinaryMessage, fullRequest); err != nil {
		fmt.Printf("发送请求失败: %v\n", err)
		return
	}

	// 读取初始响应
	_, response, err := conn.ReadMessage()
	if err != nil {
		fmt.Printf("读取响应失败: %v\n", err)
		return
	}

	parseResponse(response)

	// 发送音频数据 - 不压缩
	fmt.Println("\n发送音频数据...")

	// 音频消息头 - 与服务器相同
	audioHeader := make([]byte, 4)
	audioHeader[0] = (1 << 4) | 1                                // version 1 + header size 1
	audioHeader[1] = (clientAudioRequest << 4) | noSequence      // message type + flags
	audioHeader[2] = (0 << 4) | 0                                // no serialization, no compression
	audioHeader[3] = 0                                           // reserved
	audioSize := make([]byte, 4)
	binary.BigEndian.PutUint32(audioSize, uint32(len(audioData)))
	audioMessage := append(audioHeader, audioSize...)
	audioMessage = append(audioMessage, audioData...)

	if err := conn.WriteMessage(websocket.BinaryMessage, audioMessage); err != nil {
		fmt.Printf("发送音频失败: %v\n", err)
		return
	}
	fmt.Printf("音频已发送 (%d bytes)\n", len(audioData))

	// 发送结束标记 - flags=2 (negSequence) 表示最后一条
	fmt.Println("发送结束标记...")
	endHeader := make([]byte, 4)
	endHeader[0] = (1 << 4) | 1                                 // version 1 + header size 1
	endHeader[1] = (clientAudioRequest << 4) | negSequence      // message type + flags
	endHeader[2] = (0 << 4) | 0                                 // no serialization, no compression
	endHeader[3] = 0                                            // reserved
	conn.WriteMessage(websocket.BinaryMessage, endHeader)

	// 读取结果
	fmt.Println("等待识别结果...")
	for i := 0; i < 10; i++ {
		_, resp, err := conn.ReadMessage()
		if err != nil {
			fmt.Printf("接收失败: %v\n", err)
			break
		}
		parseResponse(resp)
	}
}

func parseResponse(data []byte) {
	if len(data) < 4 {
		return
	}

	messageType := data[1] >> 4
	headSize := data[0] & 0x0f
	payload := data[headSize*4:]

	switch messageType {
	case 0x9: // full response
		fmt.Printf("\n收到响应:\n")
		if payload[0] == '{' {
			var result map[string]interface{}
			json.Unmarshal(payload, &result)
			j, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(j))
		}
	case 0xb: // ack
		seq := binary.BigEndian.Uint32(payload[0:4])
		fmt.Printf("ACK seq=%d\n", seq)
	case 0xf: // error
		code := binary.BigEndian.Uint32(payload[0:4])
		errMsg := payload[8:]
		r, _ := gzip.NewReader(bytes.NewReader(errMsg))
		if r != nil {
			if d, _ := io.ReadAll(r); len(d) > 0 {
				errMsg = d
			}
			r.Close()
		}
		fmt.Printf("错误 [%d]: %s\n", code, string(errMsg))
	}
}
