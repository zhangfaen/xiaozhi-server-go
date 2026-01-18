package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/faiface/beep"
	"github.com/faiface/beep/speaker"
	"github.com/gorilla/websocket"
	opus "github.com/qrtc/opus-go"
)

// WebSocketClient WebSocket客户端
type WebSocketClient struct {
	conn          *websocket.Conn
	opusDecoder   *opus.OpusDecoder
	speakerFormat beep.Format
	pemData       []byte // 用于存储接收的PEM数据，目的是积累一些数据再播放，避免播放不连续
	mu            sync.Mutex
	done          chan struct{}
}

// NewWebSocketClient 创建新的WebSocket客户端
func NewWebSocketClient() *WebSocketClient {
	// 创建opus解码器配置
	opusConfig := &opus.OpusDecoderConfig{
		SampleRate:  24000, // 使用24kHz采样率
		MaxChannels: 1,     // 单声道
	}

	// 创建opus解码器
	decoder, err := opus.CreateOpusDecoder(opusConfig)
	if err != nil {
		log.Fatalf("创建Opus解码器失败: %v", err)
	}

	// 配置扬声器格式
	speakerFormat := beep.Format{
		SampleRate:  24000,
		NumChannels: 1,
		Precision:   2, // 16位PCM
	}

	return &WebSocketClient{
		opusDecoder:   decoder,
		speakerFormat: speakerFormat,
		pemData:       make([]byte, 0), // PCM数据缓冲区，可根据需要调整
		done:          make(chan struct{}),
	}
}

// Connect 连接到WebSocket服务器
func (c *WebSocketClient) Connect(url string, deviceID, clientID string) error {
	// 设置请求头
	headers := map[string][]string{
		"Device-Id": []string{deviceID},
		"Client-Id": []string{clientID},
	}

	var err error
	c.conn, _, err = websocket.DefaultDialer.Dial(url, headers)
	if err != nil {
		return fmt.Errorf("连接失败: %v", err)
	}

	// 初始化扬声器
	err = speaker.Init(c.speakerFormat.SampleRate, c.speakerFormat.SampleRate.N(time.Second/10))
	if err != nil {
		return fmt.Errorf("初始化扬声器失败: %v", err)
	}

	// 发送hello消息
	err = c.sendHelloMessage(deviceID, clientID)
	if err != nil {
		return fmt.Errorf("发送hello消息失败: %v", err)
	}

	return nil
}

// sendHelloMessage 发送hello消息建立连接
func (c *WebSocketClient) sendHelloMessage(deviceID, clientID string) error {
	hello := map[string]interface{}{
		"type": "hello",
		"data": map[string]interface{}{
			"device_id":    deviceID,
			"client_id":    clientID,
			"version":      "1.0.0",
			"audio_format": "pcm",
			"sample_rate":  24000,
		},
	}

	data, err := json.Marshal(hello)
	if err != nil {
		return fmt.Errorf("序列化hello消息失败: %v", err)
	}

	return c.conn.WriteMessage(websocket.TextMessage, data)
}

// SendTextMessage 发送文本消息
func (c *WebSocketClient) SendTextMessage(text string) error {
	chatMsg := map[string]interface{}{
		"type":         "chat",
		"data":         text,
		"audio_format": "pcm",
		"sample_rate":  24000,
	}

	data, err := json.Marshal(chatMsg)
	if err != nil {
		return fmt.Errorf("序列化chat消息失败: %v", err)
	}

	return c.conn.WriteMessage(websocket.TextMessage, data)
}

// StartListening 开始监听服务器消息
func (c *WebSocketClient) StartListening() {

	defer func() {
		c.Close()
	}()

	for {
		select {
		case <-c.done:
			return
		default:
			messageType, message, err := c.conn.ReadMessage()
			if err != nil {
				log.Printf("读取消息失败: %v", err)
				return
			}

			switch messageType {
			case websocket.TextMessage:
				c.handleTextMessage(message)
			case websocket.BinaryMessage:
				c.handleBinaryMessage(message)
			default:
				log.Printf("未知消息类型: %d", messageType)
			}
		}
	}
}

// handleTextMessage 处理文本消息
func (c *WebSocketClient) handleTextMessage(message []byte) {
	fmt.Printf("收到文本消息: %s\n", message)

	var msgMap map[string]interface{}
	if err := json.Unmarshal(message, &msgMap); err != nil {
		log.Printf("解析文本消息失败: %v", err)
		return
	}

	//msgType, ok := msgMap["type"].(string)
	//if !ok {
	//	log.Printf("文本消息缺少type字段: %s", string(message))
	//	return
	//}

	//switch msgType {
	//case "tts":
	//	state, _ := msgMap["state"].(string)
	//	text, _ := msgMap["text"].(string)
	//	log.Printf("TTS状态: %s, 文本: %s", state, text)
	//case "llm":
	//	text, _ := msgMap["text"].(string)
	//	emotion, _ := msgMap["emotion"].(string)
	//	log.Printf("LLM响应: %s (情绪: %s)", text, emotion)
	//default:
	//	log.Printf("收到文本消息类型: %s", msgType)
	//}
}

// handleBinaryMessage 处理二进制消息（音频数据）
func (c *WebSocketClient) handleBinaryMessage(message []byte) {
	// 解码opus数据为PCM
	decodedData, err := c.decodeOpusData(message)
	if err != nil {
		log.Printf("解码Opus数据失败: %v", err)
		return
	}
	c.pemData = append(c.pemData, decodedData...)
	if len(c.pemData) >= 48000 {
		c.playPCMData(c.pemData)
		c.pemData = make([]byte, 0)
	}
}

// decodeOpusData 解码opus数据为PCM
func (c *WebSocketClient) decodeOpusData(opusData []byte) ([]byte, error) {
	if len(opusData) == 0 {
		return nil, nil
	}

	// 输出缓冲区大小计算：24000Hz * 2字节/样本 * 1通道 * 0.12秒 = 5760字节
	outBuffer := make([]byte, 8192)

	// 解码opus数据
	n, err := c.opusDecoder.Decode(opusData, outBuffer)
	if err != nil {
		return nil, fmt.Errorf("Opus解码失败: %v", err)
	}

	return outBuffer[:n], nil
}

// playPCMData 播放PCM数据
func (c *WebSocketClient) playPCMData(pcmData []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 创建自定义Streamer来播放PCM数据
	streamer := &PCMStreamer{
		data:   pcmData,
		offset: 0,
		format: c.speakerFormat,
	}

	// 播放音频
	done := make(chan struct{})
	speaker.Play(beep.Seq(streamer, beep.Callback(func() {
		close(done)
	})))

	// 等待播放完成
	select {
	case <-done:
		// 播放完成
	case <-time.After(5 * time.Second):
		log.Println("音频播放超时")
	}
}

// PCMStreamer 自定义PCM流实现
type PCMStreamer struct {
	data   []byte
	offset int
	format beep.Format
}

// Stream 实现beep.Streamer接口
func (s *PCMStreamer) Stream(samples [][2]float64) (n int, ok bool) {
	// 计算可以处理的样本数
	available := len(s.data) - s.offset
	if available <= 0 {
		return 0, false
	}

	// 每个样本占用2字节
	samplesAvailable := available / (s.format.Precision * s.format.NumChannels)
	if samplesAvailable <= 0 {
		return 0, false
	}

	// 限制样本数不超过请求的数量
	if samplesAvailable > len(samples) {
		samplesAvailable = len(samples)
	}

	// 将PCM数据转换为float64样本
	for i := 0; i < samplesAvailable; i++ {
		// 读取16位PCM样本
		sample := int16(s.data[s.offset]) | int16(s.data[s.offset+1])<<8
		s.offset += 2

		// 转换为float64（范围：-1.0到1.0）
		fSample := float64(sample) / 32768.0

		// 填充所有声道
		for ch := range samples[i] {
			samples[i][ch] = fSample
		}
	}

	return samplesAvailable, true
}

// Err 实现beep.Streamer接口
func (s *PCMStreamer) Err() error {
	return nil
}

// Close 关闭客户端
func (c *WebSocketClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	close(c.done)

	if c.opusDecoder != nil {
		c.opusDecoder.Close()
	}

	if c.conn != nil {
		c.conn.Close()
	}

	speaker.Close()
}

func main() {
	// 创建客户端
	client := NewWebSocketClient()
	defer client.Close()

	// 连接到服务器
	url := "ws://localhost:8000" // 端口改为8000
	deviceID := "virtual-esp32s3"
	clientID := "virtual-esp32s3-001"

	log.Printf("连接到服务器: %s", url)
	err := client.Connect(url, deviceID, clientID)
	if err != nil {
		log.Fatalf("连接失败: %v", err)
	}
	log.Println("连接成功")

	// 启动消息监听
	go client.StartListening()

	// 发送测试消息
	client.SendTextMessage("你好，小智，介绍一下你自己？")

	// 读取用户输入并发送消息
	reader := bufio.NewReader(os.Stdin)
	log.Println("WebSocket客户端已启动。输入文本消息发送到服务器，输入'exit'退出。")

	for {
		fmt.Print("> ")
		text, err := reader.ReadString('\n')
		if err != nil {
			log.Printf("读取输入失败: %v", err)
			continue
		}

		// 去除换行符
		text = text[:len(text)-1]

		if text == "exit" {
			log.Println("退出客户端")
			return
		}

		// 发送消息
		err = client.SendTextMessage(text)
		if err != nil {
			log.Printf("发送消息失败: %v", err)
			continue
		}

		log.Printf("已发送消息: %s", text)
	}
}
