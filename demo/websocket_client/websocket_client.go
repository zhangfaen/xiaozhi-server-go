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
	dynamicStream *DynamicStreamer
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

	// 创建动态音频流
	dynamicStream := NewDynamicStreamer(speakerFormat)

	return &WebSocketClient{
		opusDecoder:   decoder,
		speakerFormat: speakerFormat,
		dynamicStream: dynamicStream,
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
	err = speaker.Init(c.speakerFormat.SampleRate, c.speakerFormat.SampleRate.N(time.Second/50))
	if err != nil {
		return fmt.Errorf("初始化扬声器失败: %v", err)
	}

	// 发送hello消息
	err = c.sendHelloMessage(deviceID, clientID)
	if err != nil {
		return fmt.Errorf("发送hello消息失败: %v", err)
	}

	// 启动持续播放
	speaker.Play(c.dynamicStream)

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
}

// handleBinaryMessage 处理二进制消息（音频数据）
func (c *WebSocketClient) handleBinaryMessage(message []byte) {
	// 解码opus数据为PCM
	decodedData, err := c.decodeOpusData(message)
	if err != nil {
		log.Printf("解码Opus数据失败: %v", err)
		return
	}
	// 立即将解码后的PCM数据添加到动态流
	c.dynamicStream.AddData(decodedData)
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

// DynamicStreamer 动态音频流实现，支持持续添加数据
type DynamicStreamer struct {
	format beep.Format
	data   []byte
	mu     sync.Mutex
	cond   *sync.Cond
}

// NewDynamicStreamer 创建动态音频流
func NewDynamicStreamer(format beep.Format) *DynamicStreamer {
	ds := &DynamicStreamer{
		format: format,
		data:   make([]byte, 0),
	}
	ds.cond = sync.NewCond(&ds.mu)
	return ds
}

// AddData 添加PCM数据到动态流
func (ds *DynamicStreamer) AddData(pcmData []byte) {
	if len(pcmData) == 0 {
		return
	}

	ds.mu.Lock()
	defer ds.mu.Unlock()

	ds.data = append(ds.data, pcmData...)
	ds.cond.Signal() // 通知等待的Stream方法有新数据
}

// Stream 实现beep.Streamer接口
func (ds *DynamicStreamer) Stream(samples [][2]float64) (n int, ok bool) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	bytesPerSample := ds.format.Precision * ds.format.NumChannels
	totalBytesNeeded := len(samples) * bytesPerSample

	// 如果数据不足，等待新数据
	for len(ds.data) < totalBytesNeeded {
		// 如果没有数据且需要的数据量大于0，等待
		if len(ds.data) == 0 {
			ds.cond.Wait()
		} else {
			// 有部分数据，先处理这部分
			break
		}
	}

	// 计算可以处理的样本数
	availableSamples := len(ds.data) / bytesPerSample
	if availableSamples == 0 {
		return 0, true // 继续等待新数据
	}

	// 限制样本数不超过请求的数量
	if availableSamples > len(samples) {
		availableSamples = len(samples)
	}

	// 将PCM数据转换为float64样本
	for i := 0; i < availableSamples; i++ {
		// 读取16位PCM样本
		sample := int16(ds.data[i*2]) | int16(ds.data[i*2+1])<<8

		// 转换为float64（范围：-1.0到1.0）
		fSample := float64(sample) / 32768.0

		// 填充所有声道
		for ch := range samples[i] {
			samples[i][ch] = fSample
		}
	}

	// 移除已处理的数据
	processedBytes := availableSamples * bytesPerSample
	ds.data = ds.data[processedBytes:]

	return availableSamples, true
}

// Err 实现beep.Streamer接口
func (ds *DynamicStreamer) Err() error {
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
