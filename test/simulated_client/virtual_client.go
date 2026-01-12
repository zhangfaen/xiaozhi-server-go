// +build ignore

// 虚拟 ESP32S3 客户端 - 模拟对话测试
package main

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

var (
	serverAddr = flag.String("server", "localhost:8000", "服务器地址")
	deviceID   = flag.String("device", "virtual-esp32s3", "设备ID")
	audioDir   = flag.String("audio", "audio", "音频目录")
)

type Session struct {
	conn  *websocket.Conn
	reqID string
}

func NewSession() *Session {
	return &Session{
		reqID: fmt.Sprintf("session-%d", time.Now().UnixNano()),
	}
}

func (s *Session) connect() error {
	url := fmt.Sprintf("ws://%s/ws?clientId=%s", *serverAddr, *deviceID)
	fmt.Printf("连接服务器: %s\n", url)

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return fmt.Errorf("连接失败: %v", err)
	}
	s.conn = conn
	fmt.Println("连接成功!")
	return nil
}

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

func parseASRResponse(data []byte) string {
	if len(data) < 4 {
		return ""
	}

	headerSize := int(data[0] & 0x0f)
	compressionMethod := data[2] & 0x0f

	payload := data[headerSize*4:]

	// 跳过 sequence
	if len(payload) >= 4 {
		payload = payload[4:]
	}

	// 跳过 payloadSize (4 bytes)
	if len(payload) >= 4 {
		payload = payload[4:]
	}

	// 解压
	if compressionMethod == gzipCompression {
		r, err := gzip.NewReader(bytes.NewReader(payload))
		if err == nil {
			decompressed, _ := io.ReadAll(r)
			if len(decompressed) > 0 {
				payload = decompressed
			}
			r.Close()
		}
	}

	// 查找 JSON 并解析
	payloadStr := string(payload)
	start := strings.Index(payloadStr, "{\"")
	if start < 0 {
		return ""
	}

	var result struct {
		Result struct {
			Text string `json:"text"`
		} `json:"result"`
	}
	json.Unmarshal([]byte(payloadStr[start:]), &result)
	return result.Result.Text
}

func main() {
	flag.Parse()

	fmt.Println("==========================================")
	fmt.Println("  虚拟 ESP32S3 客户端 - 对话测试")
	fmt.Println("==========================================")

	session := NewSession()

	if err := session.connect(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer session.conn.Close()

	go func() {
		for {
			_, message, err := session.conn.ReadMessage()
			if err != nil {
				break
			}
			fmt.Printf("\n收到服务器消息 (长度: %d)\n", len(message))
		}
	}()

	time.Sleep(500 * time.Millisecond)

	dialogs := []struct {
		name string
		text string
	}{
		{"user_hello_weather", "小智你好，我想问问北京今天天气怎么样"},
		{"user_tourist", "那北京有什么好玩的地方推荐吗"},
		{"user_thanks", "好的，谢谢小智，再见"},
	}

	fmt.Println("\n开始对话测试...")
	fmt.Println("------------------------------------------")

	for i, d := range dialogs {
		filePath := filepath.Join(*audioDir, d.name+".pcm")

		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			fmt.Printf("[对话 %d/%d] 文件不存在: %s\n", i+1, len(dialogs), filePath)
			continue
		}

		fmt.Printf("\n[对话 %d/%d] 用户: %s\n", i+1, len(dialogs), d.text)

		text, err := session.sendAudioToASR(filePath)
		if err != nil {
			fmt.Printf("  ASR 失败: %v\n", err)
			continue
		}

		fmt.Printf("  -> ASR 识别: %s\n", text)

		fmt.Println("  等待 AI 回复...")
		time.Sleep(2 * time.Second)
	}

	fmt.Println("\n------------------------------------------")
	fmt.Println("对话测试完成!")
	fmt.Println("==========================================")
}

func (s *Session) sendAudioToASR(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("读取音频失败: %v", err)
	}

	fmt.Printf("  发送音频: %s (%d bytes)\n", filepath.Base(filePath), len(data))

	url := "wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_nostream"
	connectID := fmt.Sprintf("virtual-%d", time.Now().UnixNano())

	headers := map[string][]string{
		"X-Api-App-Key":     {"8833371206"},
		"X-Api-Access-Key":  {"-1yBtIJ3p6l3ApQC6zZhhH_ZEkaPV5o-"},
		"X-Api-Resource-Id": {"volc.bigasr.sauc.duration"},
		"X-Api-Connect-Id":  {connectID},
	}

	conn, _, err := websocket.DefaultDialer.Dial(url, headers)
	if err != nil {
		return "", fmt.Errorf("ASR 连接失败: %v", err)
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

	var fullText string
	for i := 0; i < 10; i++ {
		_, response, err := conn.ReadMessage()
		if err != nil {
			break
		}

		text := parseASRResponse(response)
		if text != "" {
			if fullText != "" {
				fullText += " "
			}
			fullText += text
		}
	}

	return fullText, nil
}
