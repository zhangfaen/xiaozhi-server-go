package main

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/faiface/beep"
	"github.com/faiface/beep/speaker"
	"github.com/gorilla/websocket"
	opus "github.com/qrtc/opus-go"
)

// WebSocketClient WebSocket客户端,供外部调用
type WebSocketClient struct {
	conn          *websocket.Conn
	opusDecoder   *opus.OpusDecoder
	speakerFormat beep.Format
	dynamicStream *DynamicStreamer
	recorder      *AudioRecorder // 关联的录音器，用于TTS时暂停/恢复
	url           string         // 服务器地址
	deviceID      string         // 设备ID
	clientID      string         // 客户端ID
	mu            sync.Mutex
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
	}
}

// SetRecorder 设置关联的录音器（用于TTS时暂停/恢复录音）
func (c *WebSocketClient) SetRecorder(recorder *AudioRecorder) {
	c.recorder = recorder
}

// Connect 连接到WebSocket服务器
func (c *WebSocketClient) Connect(url string, deviceID, clientID string) error {
	// 保存连接参数
	c.url = url
	c.deviceID = deviceID
	c.clientID = clientID

	// 建立连接
	err := c.doConnect()
	if err != nil {
		return fmt.Errorf("连接失败: %v", err)
	}

	// 初始化扬声器，缓冲区 10ms 可降低播放延迟
	err = speaker.Init(c.speakerFormat.SampleRate, c.speakerFormat.SampleRate.N(time.Second/100))
	if err != nil {
		return fmt.Errorf("初始化扬声器失败: %v", err)
	}

	// 启动持续播放
	speaker.Play(c.dynamicStream)

	return nil
}

// isConnected 检查连接是否有效
func (c *WebSocketClient) isConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn != nil
}

// reconnect 尝试重连
func (c *WebSocketClient) reconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 关闭旧连接
	if c.conn != nil {
		c.conn.Close()
	}

	log.Println("尝试重连服务器...")

	err := c.doConnect()
	if err != nil {
		return err
	}

	log.Println("重连成功")

	// 重连后重新启动监听
	go c.StartListening()

	return nil
}

// doConnect 建立连接（供 Connect 和 reconnect 使用）
func (c *WebSocketClient) doConnect() error {
	headers := map[string][]string{
		"Device-Id": []string{c.deviceID},
		"Client-Id": []string{c.clientID},
	}

	var err error
	c.conn, _, err = websocket.DefaultDialer.Dial(c.url, headers)
	if err != nil {
		return err
	}

	// 发送hello消息
	err = c.sendHelloMessage(c.deviceID, c.clientID)
	if err != nil {
		return err
	}

	return nil
}

// sendHelloMessage 发送hello消息建立连接
func (c *WebSocketClient) sendHelloMessage(deviceID, clientID string) error {
	hello := map[string]interface{}{
		"type": "hello",
		"audio_params": map[string]interface{}{
			"format":         "opus",
			"sample_rate":    24000,
			"channels":       1,
			"frame_duration": 60,
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

// SendTextMessageToServer 发送文本消息到服务端（用于发送 listen 等控制消息）
func (c *WebSocketClient) SendTextMessageToServer(msg map[string]interface{}) error {
	// 检查连接状态
	if !c.isConnected() {
		if err := c.reconnect(); err != nil {
			return err
		}
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("序列化消息失败: %v", err)
	}
	return c.conn.WriteMessage(websocket.TextMessage, data)
}

// SendAudioMessage 发送音频数据到服务端
// audioData: Opus 编码的音频数据
func (c *WebSocketClient) SendAudioMessage(audioData []byte) error {
	if len(audioData) == 0 {
		return nil
	}

	// 检查连接状态
	if !c.isConnected() {
		if err := c.reconnect(); err != nil {
			return err
		}
	}

	// 直接发送二进制消息（服务端根据 audio_params 中的 format 判断解码方式）
	return c.conn.WriteMessage(websocket.BinaryMessage, audioData)
}

// StartListening 开始监听服务器消息
func (c *WebSocketClient) StartListening() {
	log.Println("StartListening: 监听线程启动")
	for {
		messageType, message, err := c.conn.ReadMessage()
		if err != nil {
			log.Printf("读取消息失败: %v", err)
			// 连接断开，置空连接让主程序检测到
			c.mu.Lock()
			c.conn = nil
			c.mu.Unlock()
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

// handleTextMessage 处理文本消息
func (c *WebSocketClient) handleTextMessage(message []byte) {
	fmt.Printf("收到文本消息: %s\n", message)

	var msgMap map[string]interface{}
	if err := json.Unmarshal(message, &msgMap); err != nil {
		log.Printf("解析文本消息失败: %v", err)
		return
	}

	//// 处理 TTS 消息，暂停/恢复录音以防止回声
	//msgType, ok := msgMap["type"].(string)
	//if !ok {
	//	return
	//}
	//
	//switch msgType {
	//case "tts":
	//	// 检查 state 字段
	//	// 特别注意，实际上自动应该是 state，后面加了一个1是我估计的，目的是使下面的判断代码不起作用。
	//	// 原因目前不需要通过服务器端发送音频数据的状态来控制是否能录音
	//	state, ok := msgMap["state1"].(string)
	//	if !ok {
	//		return
	//	}
	//
	//	if c.recorder == nil {
	//		return
	//	}
	//
	//	switch state {
	//	case "start":
	//		// TTS 开始播放，暂停录音
	//		fmt.Println("\n[TTS] 开始播放，暂停录音...")
	//		c.recorder.Pause()
	//	case "stop":
	//		// TTS 播放结束，恢复录音
	//		fmt.Println("\n[TTS] 播放结束，恢复录音...")
	//		c.recorder.Resume()
	//	}
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

	if c.opusDecoder != nil {
		c.opusDecoder.Close()
	}

	if c.conn != nil {
		c.conn.Close()
	}

	speaker.Close()
}

func main() {
	// 连接到服务器
	url := "ws://localhost:8000"
	deviceID := "virtual-esp32s3"
	clientID := "virtual-esp32s3-001"

	// 先创建客户端（不需要连接）
	client := NewWebSocketClient()
	defer client.Close()

	// 创建并启动录音服务
	recorder := NewAudioRecorder(client, 24000, 1, false)
	defer recorder.Shutdown()

	// 将录音器关联到客户端（用于TTS时暂停/恢复）
	client.SetRecorder(recorder)

	// 启动录音服务（只需启动一次）
	if err := recorder.Start(); err != nil {
		log.Fatalf("启动录音服务失败: %v", err)
	}

	log.Printf("连接到服务器: %s", url)
	err := client.Connect(url, deviceID, clientID)
	if err != nil {
		log.Printf("连接失败: %v，3秒后重连...", err)
		time.Sleep(3 * time.Second)
	}
	log.Println("连接成功")

	// 启动消息监听（接收服务端音频，包括TTS消息）
	go client.StartListening()

	// 等待用户按 Ctrl+C 退出
	recorder.WaitForExit()
}
