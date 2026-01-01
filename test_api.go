// +build ignore

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024 * 1024,
	WriteBufferSize: 1024 * 1024,
}

func main() {
	fmt.Println("=== 小智服务器测试 ===")
	fmt.Println("1. 测试 WebSocket 连接")
	fmt.Println("2. 测试 TTS 直接调用")
	fmt.Println("3. 退出")
	fmt.Print("请选择: ")

	var choice int
	fmt.Scanf("%d", &choice)

	switch choice {
	case 1:
		testWebSocket()
	case 2:
		testTTS()
	case 3:
		os.Exit(0)
	}
}

func testWebSocket() {
	serverIP := getServerIP()
	url := fmt.Sprintf("ws://%s:8000/ws?clientId=test-client-001", serverIP)

	fmt.Printf("\n连接到: %s\n", url)

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		fmt.Printf("连接失败: %v\n", err)
		return
	}
	defer conn.Close()

	fmt.Println("WebSocket 连接成功!")

	// 发送激活消息
	activate := map[string]interface{}{
		"type": "activate",
		"data": map[string]interface{}{
			"no_reply":       false,
			"accept_audio":   true,
			"vad_consume":    false,
			"multimodal":     true,
			"asr_rollback":   true,
			"tts_rollback":   true,
			"playback_rollback": true,
		},
	}
	conn.WriteJSON(activate)
	fmt.Println("发送 activate 消息")

	// 接收消息
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			fmt.Printf("接收消息失败: %v\n", err)
			break
		}

		var msg map[string]interface{}
		json.Unmarshal(message, &msg)
		msgType := msg["type"]

		fmt.Printf("收到消息类型: %s\n", msgType)

		// 打印完整消息
		data, _ := json.MarshalIndent(msg, "", "  ")
		fmt.Println(string(data))

		if msgType == "ready" || msgType == "error" {
			break
		}

		// 测试发送文本
		if msgType == "recognizer:start" {
			fmt.Println("\n发送测试文本...")
			textMsg := map[string]interface{}{
				"type": "text",
				"data": map[string]interface{}{
					"text": "你好",
				},
			}
			conn.WriteJSON(textMsg)
		}
	}
}

func testTTS() {
	serverIP := getServerIP()
	url := fmt.Sprintf("http://%s:8080/api/v1/tts", serverIP)

	fmt.Printf("\n测试 TTS: %s\n", url)

	// 读取配置文件中的 TTS 配置
	configBytes, err := os.ReadFile("config.yaml")
	if err != nil {
		fmt.Printf("读取配置文件失败: %v\n", err)
		return
	}

	// 这里简化处理，直接发送请求
	reqBody := map[string]interface{}{
		"text":  "你好，这是小智服务器的测试",
		"voice": "zh_female_wanwanxiaohe_moon_bigtts",
	}
	jsonBody, _ := json.Marshal(reqBody)

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		fmt.Printf("请求失败: %v\n", err)
		return
	}
	defer resp.Body.Close()

	fmt.Printf("响应状态: %s\n", resp.Status)

	// 保存音频文件
	if resp.Header.Get("Content-Type") == "audio/mpeg" || resp.Header.Get("Content-Type") == "audio/ogg" {
		out, _ := os.Create("test_output.mp3")
		io.Copy(out, resp.Body)
		out.Close()
		fmt.Println("音频已保存到 test_output.mp3")
	} else {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("响应内容: %s\n", string(body))
	}
}

func getServerIP() string {
	// 尝试多种方式获取本机 IP
	// 1. 检查环境变量
	if ip := os.Getenv("XIAOZHI_SERVER_IP"); ip != "" {
		return ip
	}

	// 2. 尝试连接服务器获取 IP
	conn, err := net.DialTimeout("tcp", "localhost:8000", time.Second)
	if err == nil {
		localAddr := conn.LocalAddr().(*net.TCPAddr)
		ip := localAddr.IP.String()
		conn.Close()
		return ip
	}

	return "localhost"
}
