// +build ignore

// 调试 ASR 响应解析
package main

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const (
	clientFullRequest  = 0x1
	clientAudioRequest = 0x2
	noSequence         = 0x0
	negSequence        = 0x2
	jsonFormat         = 0x1
	gzipCompression    = 0x1
	noSerialization    = 0x0
)

func generateHeader(messageType, flags, serializationMethod, compressionMethod uint8) []byte {
	header := make([]byte, 4)
	header[0] = (1 << 4) | 1
	header[1] = (messageType << 4) | flags
	header[2] = (serializationMethod << 4) | compressionMethod
	header[3] = 0
	return header
}

func gzipCompress(data []byte) []byte {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	w.Write(data)
	w.Close()
	return buf.Bytes()
}

func main() {
	data, _ := os.ReadFile("audio/user_hello_weather.pcm")
	fmt.Printf("音频大小: %d bytes\n", len(data))

	url := "wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_nostream"
	connectID := fmt.Sprintf("debug-%d", time.Now().UnixNano())

	headers := map[string][]string{
		"X-Api-App-Key":     {"8833371206"},
		"X-Api-Access-Key":  {"-1yBtIJ3p6l3ApQC6zZhhH_ZEkaPV5o-"},
		"X-Api-Resource-Id": {"volc.bigasr.sauc.duration"},
		"X-Api-Connect-Id":  {connectID},
	}

	conn, _, err := websocket.DefaultDialer.Dial(url, headers)
	if err != nil {
		fmt.Printf("连接失败: %v\n", err)
		return
	}
	defer conn.Close()

	reqParams := map[string]interface{}{
		"user": map[string]interface{}{"uid": connectID},
		"audio": map[string]interface{}{
			"format": "pcm", "rate": 16000, "bits": 16, "channel": 1, "language": "zh-CN",
		},
		"request": map[string]interface{}{
			"model_name": "bigmodel", "end_window_size": 300,
			"enable_punc": true, "enable_itn": true,
			"result_type": "single", "show_utterances": false,
		},
	}

	reqBytes, _ := json.Marshal(reqParams)
	compressed := gzipCompress(reqBytes)

	reqHeader := generateHeader(clientFullRequest, noSequence, jsonFormat, gzipCompression)
	reqSize := make([]byte, 4)
	binary.BigEndian.PutUint32(reqSize, uint32(len(compressed)))
	conn.WriteMessage(websocket.BinaryMessage, append(append(reqHeader, reqSize...), compressed...))

	audioHeader := generateHeader(clientAudioRequest, noSequence, noSerialization, 0x0)
	audioSize := make([]byte, 4)
	binary.BigEndian.PutUint32(audioSize, uint32(len(data)))
	conn.WriteMessage(websocket.BinaryMessage, append(append(audioHeader, audioSize...), data...))

	endHeader := generateHeader(clientAudioRequest, negSequence, noSerialization, 0x0)
	endSize := make([]byte, 4)
	binary.BigEndian.PutUint32(endSize, 0)
	conn.WriteMessage(websocket.BinaryMessage, append(endHeader, endSize...))

	for i := 0; i < 10; i++ {
		_, response, err := conn.ReadMessage()
		if err != nil {
			break
		}

		fmt.Printf("\n=== 响应 %d ===\n", i+1)

		headerSize := int(response[0] & 0x0f)
		messageType := response[1] >> 4
		serializationMethod := response[2] >> 4
		compressionMethod := response[2] & 0x0f

		fmt.Printf("Header: version=%d, headerSize=%d, messageType=0x%x, serialization=0x%x, compression=0x%x\n",
			response[0]>>4, headerSize, messageType, serializationMethod, compressionMethod)

		payload := response[headerSize*4:]
		if len(payload) >= 4 {
			seq := binary.BigEndian.Uint32(payload[0:4])
			fmt.Printf("Sequence: %d\n", seq)
			payload = payload[4:]
		}

		// 检查是否有 payloadSize
		if len(payload) >= 4 {
			payloadSize := binary.BigEndian.Uint32(payload[0:4])
			fmt.Printf("Payload size: %d\n", payloadSize)
		}

		// 解压
		payloadStr := string(payload)
		if compressionMethod == gzipCompression {
			r, err := gzip.NewReader(bytes.NewReader(payload))
			if err == nil {
				decompressed, _ := io.ReadAll(r)
				if len(decompressed) > 0 {
					payload = decompressed
					payloadStr = string(decompressed)
				}
				r.Close()
			}
		}

		fmt.Printf("Payload (raw): %q\n", payloadStr)

		// 查找 JSON
		start := strings.Index(payloadStr, "{\"")
		if start >= 0 {
			jsonStr := payloadStr[start:]
			fmt.Printf("JSON: %s\n", jsonStr[:min(200, len(jsonStr))])

			var result struct {
				Result struct {
					Text string `json:"text"`
				} `json:"result"`
			}
			json.Unmarshal([]byte(jsonStr), &result)
			fmt.Printf("提取的 text: '%s'\n", result.Result.Text)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
