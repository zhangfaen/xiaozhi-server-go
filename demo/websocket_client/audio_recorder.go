package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/gordonklaus/portaudio"
	opus "github.com/qrtc/opus-go"
)

// RecorderState 录音状态
type RecorderState int

const (
	StateIdle       RecorderState = iota // 空闲态（等待说话）
	StateRecording                       // 录音中态
	StateSubmitting                      // 提交态
	StateWaiting                         // 等待态
	StatePaused                          // 暂停态（TTS播放中）
)

// AudioRecorder 音频录制器
//
// 功能：
//   - 自动检测语音活动（VAD）
//   - 自动录音和提交
type AudioRecorder struct {
	wsClient      *WebSocketClient // WebSocket 客户端，用于发送音频和控制消息
	sampleRate    int              // 采样率（Hz），默认 24000
	numChannels   int              // 声道数，默认 1（单声道）
	frameDuration int              // 音频帧长（毫秒），默认 60ms

	// Opus 编码器
	opusEncoder *opus.OpusEncoder

	// 音频数据
	saveDir        string // 录音文件保存目录
	shouldSaveFile bool   // 是否保存录音文件
	allPcmData     []byte // 保存所有 PCM 原始数据，用于生成 WAV 文件
	aecPcmData     []byte // AEC 处理后的 PCM 数据，暂时没有任何含义，先放在这里

	// VAD 参数
	vadThreshold      int           // 语音能量阈值（0-32767）
	silenceTimeout    time.Duration // 连续静音超时时间
	minRecordDuration time.Duration // 最小录音时长
	maxRecordDuration time.Duration // 最大录音时长

	// 状态管理
	state            RecorderState // 当前状态
	silenceStartTime time.Time     // 静音开始时间
	recordStartTime  time.Time     // 录音开始时间
	waitStartTime    time.Time     // 等待开始时间

	// 麦克风
	inputDevice *portaudio.DeviceInfo

	// 控制
	done          chan struct{}
	pauseChan     chan bool // true=暂停, false=恢复
	wasRecording  bool      // 暂停前是否正在录音
	closeOnce     sync.Once
	portaudioInit bool // PortAudio 是否已初始化
}

// NewAudioRecorder 创建音频录制器
//
// 参数：
//   - wsClient: WebSocket 客户端实例，用于发送音频数据和控制消息
//   - sampleRate: 采样率（Hz），ASR 服务端要求 24000
//   - numChannels: 声道数，ASR 服务端要求 1（单声道）
func NewAudioRecorder(wsClient *WebSocketClient, sampleRate, numChannels int, shouldSaveFile bool) *AudioRecorder {
	return &AudioRecorder{
		wsClient:          wsClient,
		sampleRate:        sampleRate,
		numChannels:       numChannels,
		frameDuration:     60,
		vadThreshold:      500,
		silenceTimeout:    1000 * time.Millisecond,
		minRecordDuration: 500 * time.Millisecond,
		maxRecordDuration: 30 * time.Second,
		saveDir:           "./demo/websocket_client/recorder_files",
		state:             StateIdle,
		done:              make(chan struct{}),
		pauseChan:         make(chan bool, 1),
		shouldSaveFile:    shouldSaveFile,
	}
}

// Start 启动自动录音服务
func (ar *AudioRecorder) Start() error {
	// 初始化 PortAudio
	if err := portaudio.Initialize(); err != nil {
		return fmt.Errorf("初始化PortAudio失败: %v", err)
	}
	ar.portaudioInit = true

	// 创建 Opus 编码器
	opusConfig := &opus.OpusEncoderConfig{
		SampleRate:    ar.sampleRate,
		MaxChannels:   ar.numChannels,
		Application:   opus.AppVoIP,
		FrameDuration: opus.Framesize60Ms,
	}
	encoder, err := opus.CreateOpusEncoder(opusConfig)
	if err != nil {
		portaudio.Terminate()
		return fmt.Errorf("创建Opus编码器失败: %v", err)
	}
	ar.opusEncoder = encoder

	// 选择麦克风设备
	if err := ar.selectInputDevice(); err != nil {
		ar.Close()
		return err
	}

	// 启动主循环
	go ar.runLoop()

	return nil
}

// selectInputDevice 选择麦克风设备
func (ar *AudioRecorder) selectInputDevice() error {
	devices, err := portaudio.Devices()
	if err != nil {
		return fmt.Errorf("获取设备列表失败: %v", err)
	}

	fmt.Println("\n=== 选择麦克风设备 ===")
	inputDevices := make([]*portaudio.DeviceInfo, 0)
	for _, d := range devices {
		if d.MaxInputChannels > 0 {
			inputDevices = append(inputDevices, d)
			fmt.Printf("  [%d] %s (输入=%d)\n", len(inputDevices), d.Name, d.MaxInputChannels)
		}
	}

	if len(inputDevices) == 0 {
		return fmt.Errorf("没有可用的麦克风设备")
	}

	fmt.Print("\n选择麦克风 (直接回车使用默认设备): ")
	var choice int
	fmt.Scanf("%d", &choice)

	if choice < 1 || choice > len(inputDevices) {
		defaultDevice, err := portaudio.DefaultInputDevice()
		if err != nil {
			ar.inputDevice = inputDevices[0]
		} else {
			ar.inputDevice = defaultDevice
		}
	} else {
		ar.inputDevice = inputDevices[choice-1]
	}
	fmt.Printf("使用设备: %s\n", ar.inputDevice.Name)
	return nil
}

// runLoop 主循环
func (ar *AudioRecorder) runLoop() {
	defer ar.Close()

	fmt.Println("\n=== 自动录音服务已启动 ===")
	fmt.Println("检测到说话自动开始录音，连续 1000ms 静音自动提交")
	fmt.Println("按 Ctrl+C 退出程序")
	fmt.Println()

	// 初始化 PortAudio 缓冲区
	chunkSize := ar.sampleRate * ar.frameDuration / 1000
	micBuffer := make([]int16, chunkSize)

	// 打开麦克风
	stream, err := portaudio.OpenStream(portaudio.StreamParameters{
		Input: portaudio.StreamDeviceParameters{
			Device:   ar.inputDevice,
			Channels: 1,
			Latency:  time.Duration(0),
		},
		Output: portaudio.StreamDeviceParameters{
			Device:   nil,
			Channels: 0,
			Latency:  time.Duration(0),
		},
		SampleRate:      float64(ar.sampleRate),
		FramesPerBuffer: chunkSize,
	}, micBuffer)
	if err != nil {
		log.Printf("打开麦克风失败: %v", err)
		return
	}
	defer stream.Close()

	if err := stream.Start(); err != nil {
		log.Printf("启动麦克风失败: %v", err)
		return
	}
	defer stream.Stop()

	log.Printf("录音配置: 采样率=%dHz, 帧长=%dms, chunkSize=%d",
		ar.sampleRate, ar.frameDuration, chunkSize)

	// 主循环
	for {
		select {
		case <-ar.done:
			return
		case pause := <-ar.pauseChan:
			// 收到暂停/恢复消息
			if pause {
				// 暂停录音
				if ar.state == StateRecording {
					ar.wasRecording = true
					fmt.Println("\n[暂停] TTS播放中，录音已暂停")
				}
				ar.state = StatePaused
			} else {
				// 恢复录音
				if ar.wasRecording {
					ar.state = StateRecording
					ar.wasRecording = false
					fmt.Println("\n[恢复] 录音已恢复")
				} else {
					ar.state = StateIdle
				}
			}
			continue
		default:
			// 暂停状态跳过录音
			if ar.state == StatePaused {
				continue
			}

			// 读取麦克风数据
			if err := stream.Read(); err != nil {
				log.Printf("读取麦克风数据失败: %v", err)
				continue
			}

			// 转换为字节（小端序）
			pcmBytes := make([]byte, len(micBuffer)*2)
			for i, sample := range micBuffer {
				pcmBytes[i*2] = byte(sample & 0xFF)
				pcmBytes[i*2+1] = byte(sample >> 8)
			}

			// 计算音频能量
			energy := ar.calculateEnergy(pcmBytes)

			// 状态机处理
			ar.handleState(energy, pcmBytes)
		}
	}
}

// calculateEnergy 计算音频能量（平均绝对值）
func (ar *AudioRecorder) calculateEnergy(pcmData []byte) int {
	if len(pcmData) < 2 {
		return 0
	}

	total := 0
	sampleCount := len(pcmData) / 2

	for i := 0; i < sampleCount; i++ {
		sample := int16(pcmData[i*2+1])<<8 | int16(pcmData[i*2]&0xFF)
		if sample < 0 {
			total += -int(sample)
		} else {
			total += int(sample)
		}
	}

	if sampleCount > 0 {
		return total / sampleCount
	}
	return 0
}

// handleState 状态机处理
func (ar *AudioRecorder) handleState(energy int, pcmData []byte) {
	now := time.Now()

	switch ar.state {
	case StateIdle:
		// 检测到语音，开始录音
		if energy > ar.vadThreshold {
			ar.state = StateRecording
			ar.recordStartTime = now
			ar.allPcmData = nil
			ar.aecPcmData = nil

			// 发送 listen start
			ar.sendListenMessage("start")

			fmt.Println("\n[录音中...] (检测到语音)")
		}

	case StateRecording:
		// 检查最大录音时长
		if now.Sub(ar.recordStartTime) > ar.maxRecordDuration {
			ar.submitRecording()
			return
		}

		// 检查是否静音
		if energy < ar.vadThreshold {
			if ar.silenceStartTime.IsZero() {
				ar.silenceStartTime = now
			} else if now.Sub(ar.silenceStartTime) >= ar.silenceTimeout {
				// 连续静音超时，检查是否达到最小录音时长
				if now.Sub(ar.recordStartTime) >= ar.minRecordDuration {
					ar.submitRecording()
					return
				}
			}
		} else {
			// 有语音，重置静音计时器
			ar.silenceStartTime = time.Time{}
		}

		// 累积数据并发送
		ar.allPcmData = append(ar.allPcmData, pcmData...)
		ar.encodeAndSend(pcmData)

	case StateSubmitting:
		// 提交完成，进入等待态
		ar.state = StateWaiting
		ar.waitStartTime = now
		fmt.Println("[音频数据提交完成]")

	case StateWaiting:
		// 等待超时后回到空闲态
		if now.Sub(ar.waitStartTime) >= 500*time.Millisecond {
			ar.state = StateIdle
			ar.silenceStartTime = time.Time{}
		}
	default:
		panic("unhandled default case")
	}
}

// submitRecording 提交录音
func (ar *AudioRecorder) submitRecording() {
	ar.state = StateSubmitting

	// 发送 listen stop
	ar.sendListenMessage("stop")

	// 保存录音文件
	if len(ar.allPcmData) > 0 && ar.shouldSaveFile {
		ar.saveRecording(ar.allPcmData)
	}
}

// sendListenMessage 发送 listen 消息
func (ar *AudioRecorder) sendListenMessage(state string) {
	msg := map[string]interface{}{
		"type":  "listen",
		"state": state,
		"mode":  "realtime",
	}
	ar.wsClient.SendTextMessageToServer(msg)
}

// encodeAndSend Opus 编码并发送
func (ar *AudioRecorder) encodeAndSend(pcmData []byte) {
	outBuf := make([]byte, 128)
	n, err := ar.opusEncoder.Encode(pcmData, outBuf)
	if err != nil {
		log.Printf("Opus编码失败: %v", err)
		return
	}

	if err := ar.wsClient.SendAudioMessage(outBuf[:n]); err != nil {
		log.Printf("发送音频数据失败: %v", err)
	}
}

// saveRecording 保存录音到 WAV 文件
func (ar *AudioRecorder) saveRecording(pcmData []byte) {
	// 确保目录存在
	if err := os.MkdirAll(ar.saveDir, 0755); err != nil {
		log.Printf("创建保存目录失败: %v", err)
		return
	}

	// 生成文件名
	timestamp := time.Now().Format("20060102_150405")
	filename := filepath.Join(ar.saveDir, fmt.Sprintf("recording_%s.wav", timestamp))

	// 保存 WAV 文件
	if err := ar.writeWavFile(filename, pcmData); err != nil {
		log.Printf("保存录音文件失败: %v", err)
		return
	}

	// 计算时长
	duration := float64(len(pcmData)) / float64(ar.sampleRate*ar.numChannels*2)
	log.Printf("录音已保存: %s (%.1f秒)", filename, duration)
}

// writeWavFile 保存为 WAV 格式
func (ar *AudioRecorder) writeWavFile(filename string, pcmData []byte) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	sampleRate := ar.sampleRate
	numChannels := ar.numChannels
	bitsPerSample := 16
	byteRate := sampleRate * numChannels * bitsPerSample / 8
	blockAlign := numChannels * bitsPerSample / 8
	dataSize := len(pcmData)
	fileSize := 36 + dataSize

	// 写入 WAV 头
	header := make([]byte, 44)
	header[0], header[1], header[2], header[3] = 'R', 'I', 'F', 'F'
	header[4] = byte(fileSize)
	header[5] = byte(fileSize >> 8)
	header[6] = byte(fileSize >> 16)
	header[7] = byte(fileSize >> 24)
	header[8], header[9], header[10], header[11] = 'W', 'A', 'V', 'E'
	header[12], header[13], header[14], header[15] = 'f', 'm', 't', ' '
	header[16] = 16
	header[20] = 1
	header[22] = byte(numChannels)
	header[24] = byte(sampleRate)
	header[25] = byte(sampleRate >> 8)
	header[26] = byte(sampleRate >> 16)
	header[27] = byte(sampleRate >> 24)
	header[28] = byte(byteRate)
	header[29] = byte(byteRate >> 8)
	header[30] = byte(byteRate >> 16)
	header[31] = byte(byteRate >> 24)
	header[32] = byte(blockAlign)
	header[34] = byte(bitsPerSample)
	header[36], header[37], header[38], header[39] = 'd', 'a', 't', 'a'
	header[40] = byte(dataSize)
	header[41] = byte(dataSize >> 8)
	header[42] = byte(dataSize >> 16)
	header[43] = byte(dataSize >> 24)

	if _, err := file.Write(header); err != nil {
		return err
	}
	if _, err := file.Write(pcmData); err != nil {
		return err
	}

	return nil
}

// Pause 暂停录音（TTS播放时调用）
func (ar *AudioRecorder) Pause() {
	select {
	case ar.pauseChan <- true:
	default:
		// 通道已满，跳过
	}
}

// Resume 恢复录音（TTS播放结束时调用）
func (ar *AudioRecorder) Resume() {
	select {
	case ar.pauseChan <- false:
	default:
		// 通道已满，跳过
	}
}

// Close 关闭录制器（不终止 PortAudio，由 Shutdown 统一处理）
func (ar *AudioRecorder) Close() {
	ar.closeOnce.Do(func() {
		// 安全关闭 done 通道
		select {
		case <-ar.done:
			// 已关闭，不重复关闭
		default:
			close(ar.done)
		}

		if ar.isRecording() {
			ar.submitRecording()
		}

		if ar.opusEncoder != nil {
			ar.opusEncoder.Close()
		}
	})
}

// Shutdown 完全关闭（终止 PortAudio）
func (ar *AudioRecorder) Shutdown() {
	ar.Close()
	if ar.portaudioInit {
		portaudio.Terminate()
	}
}

// WaitForExit 等待用户按 Ctrl+C 退出
func (ar *AudioRecorder) WaitForExit() {
	// 等待 Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	fmt.Println("\n收到退出信号，正在关闭...")

	ar.Shutdown()
}

// isRecording 是否正在录音
func (ar *AudioRecorder) isRecording() bool {
	return ar.state == StateRecording
}
