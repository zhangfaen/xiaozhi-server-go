// +build ignore

// 测试火山引擎 ASR - 与小智服务器协议一致
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

// Protocol constants - 与小智服务器完全一致
const (
	clientFullRequest   = 0x1
	clientAudioRequest  = 0x2
	noSequence          = 0x0
	negSequence         = 0x2
	jsonFormat          = 0x1
	thriftFormat        = 0x3
	gzipCompression     = 0x1
	customCompression   = 0xF
	noSerialization     = 0x0
)

const (
	appid       = "8833371206"
	accessToken = "-1yBtIJ3p6l3ApQC6zZhhH_ZEkaPV5o-"
)

func main() {
	fmt.Println("=== 测试火山引擎 ASR (与小智服务器协议一致) ===")
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

	// 测试 ASR
	testASR(audioData)
}

func generateHeader(messageType uint8, flags uint8, serializationMethod uint8, compressionMethod uint8) []byte {
	header := make([]byte, 4)
	header[0] = (1 << 4) | 1                                 // 协议版本(4位) + 头大小(4位)
	header[1] = (messageType << 4) | flags                   // 消息类型(4位) + 消息标志(4位)
	header[2] = (serializationMethod << 4) | compressionMethod // 序列化方法(4位) + 压缩方法(4位)
	header[3] = 0                                            // 保留字段
	return header
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

	// 构造初始请求 (与小智服务器完全一致)
	reqParams := map[string]interface{}{
		"user": map[string]interface{}{
			"uid": connectID,
		},
		"audio": map[string]interface{}{
			"format":   "pcm",
			"rate":     16000,
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
	fmt.Printf("请求参数: %s\n", string(reqBytes))

	// 压缩请求
	var reqBuf bytes.Buffer
	gzipWriter := gzip.NewWriter(&reqBuf)
	gzipWriter.Write(reqBytes)
	gzipWriter.Close()
	compressedReq := reqBuf.Bytes()

	// 构造完整请求 - 与小智服务器完全一致
	reqHeader := generateHeader(clientFullRequest, noSequence, jsonFormat, gzipCompression)
	reqSize := make([]byte, 4)
	binary.BigEndian.PutUint32(reqSize, uint32(len(compressedReq)))
	fullRequest := append(reqHeader, reqSize...)
	fullRequest = append(fullRequest, compressedReq...)

	// 发送请求
	if err := conn.WriteMessage(websocket.BinaryMessage, fullRequest); err != nil {
		fmt.Printf("发送请求失败: %v\n", err)
		return
	}
	fmt.Println("请求已发送")

	// 读取初始响应
	_, response, err := conn.ReadMessage()
	if err != nil {
		fmt.Printf("读取响应失败: %v\n", err)
		return
	}
	parseResponse(response)

	// 发送音频数据 (不使用压缩)
	fmt.Println("\n发送音频数据...")

	// 发送音频 - 不使用压缩
	audioHeader := generateHeader(clientAudioRequest, noSequence, noSerialization, 0x0) // 无压缩
	audioSize := make([]byte, 4)
	binary.BigEndian.PutUint32(audioSize, uint32(len(audioData)))
	audioMessage := append(audioHeader, audioSize...)
	audioMessage = append(audioMessage, audioData...)

	fmt.Printf("发送音频: header=%v, size=%d, data=%d\n", audioHeader, len(audioSize), len(audioData))

	if err := conn.WriteMessage(websocket.BinaryMessage, audioMessage); err != nil {
		fmt.Printf("发送音频失败: %v\n", err)
		return
	}
	fmt.Println("音频已发送")

	// 发送结束标记 - flags=negSequence
	// 先发送一个空音频帧作为结束（模拟真实客户端行为）
	fmt.Println("发送结束帧...")

	// 发送空音频帧（isLast=true）
	endHeader := generateHeader(clientAudioRequest, negSequence, noSerialization, 0x0) // 无压缩
	endSize := make([]byte, 4)
	binary.BigEndian.PutUint32(endSize, 0)  // 空数据，大小为0
	endMessage := append(endHeader, endSize...)

	// 不追加任何音频数据，直接发送
	if err := conn.WriteMessage(websocket.BinaryMessage, endMessage); err != nil {
		fmt.Printf("发送结束帧失败: %v\n", err)
		return
	}
	fmt.Println("结束帧已发送（空音频，isLast=true）")

	// 读取结果
	fmt.Println("\n等待识别结果...")
	for i := 0; i < 20; i++ {
		_, resp, err := conn.ReadMessage()
		if err != nil {
			fmt.Printf("接收失败: %v\n", err)
			break
		}
		fmt.Printf("\n--- 收到消息 %d ---\n", i+1)
		parseResponse(resp)
	}
}

func parseResponse(data []byte) {
	if len(data) < 4 {
		fmt.Println("数据太短")
		return
	}

	headerSize := int(data[0] & 0x0f)
	messageType := data[1] >> 4
	messageTypeSpecificFlags := data[1] & 0x0f
	serializationMethod := data[2] >> 4
	compressionMethod := data[2] & 0x0f

	fmt.Printf("Header: version=%d, headerSize=%d, messageType=0x%x, flags=0x%x, serialization=0x%x, compression=0x%x\n",
		data[0]>>4, headerSize, messageType, messageTypeSpecificFlags, serializationMethod, compressionMethod)

	// 跳过头部和 sequence/ack 字段获取 payload
	payload := data[headerSize*4:]

	// 如果有 sequence number (flags bit 0)
	if messageTypeSpecificFlags&0x01 != 0 {
		if len(payload) >= 4 {
			seq := binary.BigEndian.Uint32(payload[0:4])
			fmt.Printf("  Sequence: %d\n", seq)
			payload = payload[4:]
		}
	}

	// 解压 payload (如果需要)
	if compressionMethod == 0x1 {
		r, err := gzip.NewReader(bytes.NewReader(payload))
		if err == nil {
			if decompressed, err := io.ReadAll(r); err == nil {
				payload = decompressed
				fmt.Printf("  Decompressed size: %d\n", len(payload))
			}
			r.Close()
		}
	}

	// 处理响应
	switch messageType {
	case 0x9: // full response
		fmt.Println("Full Response:")
		fmt.Printf("Payload (first 200 chars): %s\n", string(payload[:min(200, len(payload))]))
		if len(payload) > 0 && payload[0] == '{' {
			var result map[string]interface{}
			if err := json.Unmarshal(payload, &result); err == nil {
				j, _ := json.MarshalIndent(result, "", "  ")
				fmt.Println(string(j))
			}
		}
	case 0xb: // ack
		if len(payload) >= 4 {
			seq := binary.BigEndian.Uint32(payload[0:4])
			fmt.Printf("ACK: seq=%d\n", seq)
		}
	case 0xf: // error
		if len(payload) >= 8 {
			code := binary.BigEndian.Uint32(payload[0:4])
			errMsg := payload[8:]
			fmt.Printf("Error [%d]: %s\n", code, string(errMsg))
		}
	default:
		fmt.Printf("Unknown message type: 0x%x\n", messageType)
		fmt.Printf("Payload (first 200 bytes): %s\n", string(payload[:min(200, len(payload))]))
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
