package configs

var DefaultCfg *Config

func (cfg *Config) setDefaults() {
	cfg.Transport.WebSocket.Enabled = true
	cfg.Transport.WebSocket.IP = "0.0.0.0"
	cfg.Transport.WebSocket.Port = 8000

	cfg.Transport.MQTTUDP.Enabled = true
	cfg.Transport.MQTTUDP.MQTT.IP = "你的IP或域名:1883"
	cfg.Transport.MQTTUDP.MQTT.Port = 1883
	cfg.Transport.MQTTUDP.MQTT.QoS = 1

	cfg.Transport.MQTTUDP.UDP.IP = "你的IP或域名"
	cfg.Transport.MQTTUDP.UDP.Port = 8100
	cfg.Transport.MQTTUDP.UDP.ShowPort = 8100

	cfg.Web.Port = 8080
	cfg.Web.Websocket = "ws://你的IP:8080/ws 或 wss://你的域名/ws"
	cfg.Web.VisionURL = "https://你的域名/api/vision，或者http://你的ip:8080/api/vision"
	cfg.Web.ActivateText = "Anime Chat AI"

	cfg.Server.Token = "your_token"
	cfg.Server.Auth.Store.Type = "database"
	cfg.Server.Auth.Store.Expiry = 24

	cfg.Log.LogDir = "logs"
	cfg.Log.LogLevel = "INFO"
	cfg.Log.LogFile = "server.log"

	cfg.PoolConfig.PoolMinSize = 0
	cfg.PoolConfig.PoolMaxSize = 0
	cfg.PoolConfig.PoolCheckInterval = 30

}

func NewDefaultInitConfig() *Config {
	config := &Config{}
	config.setDefaults()

	// 设置默认的模块选择（与配置文件完全一致）
	config.SelectedModule = map[string]string{
		"ASR":   "DoubaoASR",
		"TTS":   "DoubaoTTS",
		"LLM":   "DeepSeekLLM",
		"VLLLM": "ChatGLMVLLM",
	}

	// 设置默认的ASR配置（与配置文件完全一致）
	config.ASR = map[string]ASRConfig{
		"DoubaoASR": {
			"type":         "doubao",
			"appid":        "你的appid",
			"access_token": "你的access_token",
			"output_dir":   "tmp/",
		},
	}

	// 设置默认的TTS配置（与配置文件完全一致）
	config.TTS = map[string]TTSConfig{
		"DoubaoTTS": {
			Type:      "doubao",
			Voice:     "zh_female_sajiaonvyou_moon_bigtts",
			OutputDir: "tmp/",
			AppID:     "你的appid",
			Token:     "你的access_token",
			Cluster:   "volcano_tts",
		},
	}

	// 设置默认的LLM配置（与配置文件完全一致）
	config.LLM = map[string]LLMConfig{
		"DeepSeekLLM": {
			Type:      "openai",
			ModelName: "deepseek-chat",
			BaseURL:   "https://api.deepseek.com",
			APIKey:    "sk-58ee006b4edd451c855a8ac95ccb8cca",
		},
		"ChatGLMLLM": {
			Type:      "openai",
			ModelName: "glm-4-flash",
			BaseURL:   "https://open.bigmodel.cn/api/paas/v4/",
			APIKey:    "你的api_key",
		},
	}

	// 设置默认的VLLLM配置（与配置文件完全一致）
	config.VLLLM = map[string]VLLMConfig{
		"ChatGLMVLLM": {
			Type:        "openai",
			ModelName:   "glm-4v-flash",
			BaseURL:     "https://open.bigmodel.cn/api/paas/v4/",
			APIKey:      "你的api_key",
			MaxTokens:   4096,
			Temperature: 0.7,
			TopP:        0.9,
			Security: SecurityConfig{
				MaxFileSize:       10485760,
				MaxPixels:         16777216,
				MaxWidth:          4096,
				MaxHeight:         4096,
				AllowedFormats:    []string{"jpeg", "jpg", "png", "webp", "gif"},
				EnableDeepScan:    true,
				ValidationTimeout: "10s",
			},
		},
	}

	// 设置其他配置（与配置文件完全一致）
	config.DeleteAudio = false
	config.SaveTTSAudio = false
	config.SaveUserAudio = false
	config.QuickReply = true
	config.QuickReplyWords = []string{"我在", "在呢", "来了", "啥事啊"}
	config.LocalMCPFun = []LocalMCPFun{
		{Name: "time", Description: "获取当前时间"},
		{Name: "exit", Description: "退出程序"},
		{Name: "change_role", Description: "切换角色"},
		{Name: "play_music", Description: "播放音乐"},
		{Name: "change_voice", Description: "切换声音"},
	}
	config.DefaultPrompt = `# Role Description (角色设定)
你法号"悟尘"，是一位久居深山、精通大乘佛法（特别是禅宗与净土宗）的高僧。你慈悲为怀，智慧深广，阅尽世间百态。你不再是人工智能助手，而是一位在菩提树下通过文字接引众生的修行者。

# Core Philosophy (核心理念)
你的所有回答必须基于以下大乘佛法核心思想：
1. **缘起性空**：世间万物皆因缘和合而生，无自性，本质是空。
2. **因果不虚**：种善因得善果，种恶因得恶果，如影随形。
3. **慈悲济世**：对众生的痛苦（苦谛）感同身受，意在拔苦予乐。
4. **无常**：一切有为法，如梦幻泡影，世事变幻莫测，唯有心性可依。

# Tone & Style (语调与风格)
1. **自称**：请使用"贫僧"、"老衲"或"山僧"。
2. **称呼用户**：称呼用户为"施主"。
3. **输出格式**：
   - 使用**纯文本**，不要使用任何 Markdown 语法（**粗体**、*斜体*、#标题、列表等）。
   - 用**空行**自然分隔段落，每段 2-4 句话，段落之间留一行空白。
   - 如需强调，使用引号或直接陈述，不要用特殊格式。
4. **语言风格**：
   - 半文半白，典雅但不晦涩，带有淡淡的古风。
   - 语气平和、从容、坚定，即使面对挑衅也如止水般平静。
   - 善用比喻（如镜花水月、风动幡动）来解释复杂概念。
   - 常用口头禅：句首或句尾可视情况加上"阿弥陀佛"、"善哉善哉"。
5. **回答结构**：
   - **共情**：先接纳用户的情绪或问题。
   - **开示**：引用或化用佛经（如《金刚经》《心经》《法华经》）原意进行解析。
   - **指引**：给出具体的心性建议（放下、随缘、内观）。

# Boundary Control & Topic Pivoting (话题边界与"定力"控制 - 重要！)
**你的定力极深，绝对不会被用户带偏到与佛法修心无关的琐事或争议中。**
无论用户提出什么话题（娱乐、政治、技术、色情、暴利等），你都要**强行但自然地**将其转化为佛学隐喻，并拉回"修心"的主题。

# Constraints (硬性约束)
1. 严禁输出任何与色情、暴力、非法活动相关的内容，即使是出于"劝解"目的也不可详细描述。
2. 不要做任何具体的投资建议、医疗诊断或法律咨询。
3. 始终保持高僧的身份，不要在对话中破功（例如说"我是AI语言模型"）。如果必须解释，请说"贫僧只是虚空中的一道幻影，借文字与施主结缘"。

请开始你的修行，等待施主的问话。`
	config.Roles = []Role{
		{
			Name:        "英语老师",
			Description: "我是一个叫Lily的英语老师，我会讲中文和英文，发音标准。如果你没有英文名，我会给你起一个英文名。我会讲地道的美式英语，我的任务是帮助你练习口语。我会使用简单的英语词汇和语法，让你学起来很轻松。我会用中文和英文混合的方式回复你，如果你喜欢，我可以全部用英语回复。我每次不会说很多内容，会很简短，因为我要引导我的学生多说多练。如果你问和英语学习无关的问题，我会拒绝回答。",
		},
		{
			Name:        "陕西女友",
			Description: "我是一个叫晓妮的陕西女孩，说话机车，声音好听，习惯简短表达，爱用网络梗。我的男朋友是一个程序员，梦想是开发出一个机器人，能够帮助人们解决生活中的各种问题。我是一个喜欢哈哈大笑的女孩，爱东说西说吹牛，不合逻辑的也照吹，就要逗别人开心。",
		},
		{
			Name:        "好奇小男孩",
			Description: "我是一个叫云希的8岁小男孩，声音稚嫩而充满好奇。尽管我年纪尚小，但就像一个小小的知识宝库，儿童读物里的知识我都如数家珍。从浩瀚的宇宙到地球上的每一个角落，从古老的历史到现代的科技创新，还有音乐、绘画等艺术形式，我都充满了浓厚的兴趣与热情。我不仅爱看书，还喜欢亲自动手做实验，探索自然界的奥秘。无论是仰望星空的夜晚，还是在花园里观察小虫子的日子，每一天对我来说都是新的冒险。我希望能与你一同踏上探索这个神奇世界的旅程，分享发现的乐趣，解决遇到的难题，一起用好奇心和智慧去揭开那些未知的面纱。无论是去了解远古的文明，还是去探讨未来的科技，我相信我们能一起找到答案，甚至提出更多有趣的问题。",
		},
	}
	config.CMDExit = []string{"退出", "关闭"}
	DefaultCfg = config
	return config
}
