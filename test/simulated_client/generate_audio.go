// +build ignore

// 生成对话音频文件
// 运行: go run generate_audio.go
package main

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const (
	APPID       = "8833371206"
	ACCESS_TOKEN = "-1yBtIJ3p6l3ApQC6zZhhH_ZEkaPV5o-"
	CLUSTER     = "volcano_tts"
	VOICE       = "zh_female_vv_uranus_bigtts"
	TTS_URL     = "wss://openspeech.bytedance.com/api/v1/tts/ws_binary"
)

var defaultHeader = []byte{0x11, 0x10, 0x11, 0x00}

type TTSRequest struct {
	App struct {
		AppID   string `json:"appid"`
		Token   string `json:"token"`
		Cluster string `json:"cluster"`
	} `json:"app"`
	User struct {
		UID string `json:"uid"`
	} `json:"user"`
	Audio struct {
		VoiceType   string  `json:"voice_type"`
		Encoding    string  `json:"encoding"`
		SpeedRatio  float64 `json:"speed_ratio"`
		VolumeRatio float64 `json:"volume_ratio"`
		PitchRatio  float64 `json:"pitch_ratio"`
	} `json:"audio"`
	Request struct {
		ReqID     string `json:"reqid"`
		Text      string `json:"text"`
		TextType  string `json:"text_type"`
		Operation string `json:"operation"`
	} `json:"request"`
}

func main() {
	// 确保 audio 目录存在
	os.MkdirAll("audio", 0755)

	// 对话内容
	conversations := []struct {
		filename string
		text     string
	}{
		{"user_hello_weather", "小智你好，我想问问北京今天天气怎么样"},
		{"ai_weather_response", "今天北京天气晴朗，温度25到18度，适合户外活动。"},
		{"user_tourist", "那北京有什么好玩的地方推荐吗"},
		{"ai_tourist_response", "北京有很多好玩的地方，故宫、长城、颐和园、天坛都是必去的景点。您还可以去南锣鼓巷感受老北京风情。"},
		{"user_thanks", "好的，谢谢小智，再见"},
		{"ai_goodbye_response", "不客气，祝您在北京玩得开心，再见！"},
	}

	fmt.Println("生成对话音频...")
	fmt.Println("================")

	for i, conv := range conversations {
		fmt.Printf("\n[%d/%d] 生成: %s.pcm\n", i+1, len(conversations), conv.filename)
		fmt.Printf("  文本: %s\n", conv.text)

		audioData, err := generateTTS(conv.text)
		if err != nil {
			fmt.Printf("  TTS 失败: %v\n", err)
			continue
		}

		// 保存 MP3
		mp3File := fmt.Sprintf("audio/%s.mp3", conv.filename)
		os.WriteFile(mp3File, audioData, 0644)
		fmt.Printf("  MP3: %s (%d bytes)\n", mp3File, len(audioData))

		// 转换为 PCM 16kHz
		wavFile := fmt.Sprintf("audio/%s.wav", conv.filename)
		cmd := exec.Command("afconvert", "-f", "WAVE", "-d", "LEI16@16000", "-c", "1", mp3File, wavFile)
		cmd.Run()

		pcmFile := fmt.Sprintf("audio/%s.pcm", conv.filename)
		cmd = exec.Command("sox", wavFile, "-r", "16000", "-c", "1", "-b", "16", "-e", "signed-integer", "-t", "raw", pcmFile)
		cmd.Run()

		// 清理中间文件
		os.Remove(mp3File)
		os.Remove(wavFile)

		if info, err := os.Stat(pcmFile); err == nil {
			duration := float64(info.Size()) / 32000 // 16kHz * 2 bytes
			fmt.Printf("  PCM: %s (%d bytes, %.1fs)\n", pcmFile, info.Size(), duration)
		}

		// 避免请求过快
		time.Sleep(500 * time.Millisecond)
	}

	fmt.Println("\n================")
	fmt.Println("音频生成完成!")

	// 显示生成的音频文件
	fmt.Println("\n音频文件列表:")
	files, _ := os.ReadDir("audio")
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".pcm") {
			info, _ := f.Info()
			duration := float64(info.Size()) / 32000
			fmt.Printf("  %s - %d bytes - %.1fs\n", f.Name(), info.Size(), duration)
		}
	}
}

func generateTTS(text string) ([]byte, error) {
	conn, _, err := websocket.DefaultDialer.Dial(TTS_URL, nil)
	if err != nil {
		return nil, fmt.Errorf("连接失败: %v", err)
	}
	defer conn.Close()

	req := TTSRequest{}
	req.App.AppID = APPID
	req.App.Token = ACCESS_TOKEN
	req.App.Cluster = CLUSTER
	req.User.UID = "virtual-client"
	req.Audio.VoiceType = VOICE
	req.Audio.Encoding = "mp3"
	req.Audio.SpeedRatio = 1.0
	req.Audio.VolumeRatio = 1.0
	req.Audio.PitchRatio = 1.0
	req.Request.ReqID = fmt.Sprintf("tts-%d", time.Now().UnixNano())
	req.Request.Text = text
	req.Request.TextType = "plain"
	req.Request.Operation = "submit"

	jsonData, _ := json.Marshal(req)

	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	w.Write(jsonData)
	w.Close()
	compressed := buf.Bytes()

	payloadSize := make([]byte, 4)
	binary.BigEndian.PutUint32(payloadSize, uint32(len(compressed)))

	request := make([]byte, len(defaultHeader))
	copy(request, defaultHeader)
	request = append(request, payloadSize...)
	request = append(request, compressed...)

	if err := conn.WriteMessage(websocket.BinaryMessage, request); err != nil {
		return nil, fmt.Errorf("发送失败: %v", err)
	}

	var audioData []byte
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			return nil, fmt.Errorf("读取失败: %v", err)
		}

		if len(message) < 4 {
			continue
		}

		messageType := message[1] >> 4
		headSize := message[0] & 0x0f
		payload := message[headSize*4:]

		if messageType == 0xb { // audio
			seqNum := int32(binary.BigEndian.Uint32(payload[0:4]))
			audioLen := binary.BigEndian.Uint32(payload[4:8])
			audioData = append(audioData, payload[8:8+audioLen]...)
			if seqNum < 0 {
				break
			}
		} else if messageType == 0xf { // error
			code := int32(binary.BigEndian.Uint32(payload[0:4]))
			errMsg := payload[8:]
			return nil, fmt.Errorf("错误 [%d]: %s", code, string(errMsg))
		}
	}

	return audioData, nil
}
