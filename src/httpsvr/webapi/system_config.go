package webapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/configs/database"
	"xiaozhi-server-go/src/core/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// SystemConfigService 系统配置服务
type SystemConfigService struct {
	logger *utils.Logger
	db     *gorm.DB
}

// NewSystemConfigService 构造函数
func NewSystemConfigService(logger *utils.Logger, db *gorm.DB) *SystemConfigService {
	return &SystemConfigService{
		logger: logger,
		db:     db,
	}
}

// RegisterRoutes 注册路由
func (s *SystemConfigService) RegisterRoutes(apiGroup *gin.RouterGroup) {
	// 需要管理员权限的配置管理路由
	adminGroup := apiGroup.Group("/admin/config")
	adminGroup.Use(AuthMiddleware(), AdminMiddleware())
	{
		// 应用配置
		adminGroup.GET("/application", s.handleGetApplicationConfig)
		adminGroup.PUT("/application", s.handleUpdateApplicationConfig)

		// 认证配置
		adminGroup.GET("/auth", s.handleGetAuthConfig)
		adminGroup.PUT("/auth", s.handleUpdateAuthConfig)

		// 传输配置
		adminGroup.GET("/transport", s.handleGetTransportConfig)
		adminGroup.PUT("/transport", s.handleUpdateTransportConfig)

		// Web配置
		adminGroup.GET("/web", s.handleGetWebConfig)
		adminGroup.PUT("/web", s.handleUpdateWebConfig)

		// 日志配置
		adminGroup.GET("/log", s.handleGetLogConfig)
		adminGroup.PUT("/log", s.handleUpdateLogConfig)

		// 角色配置
		adminGroup.GET("/roles", s.handleGetRoleConfigs)
		adminGroup.POST("/roles", s.handleCreateRoleConfig)
		adminGroup.PUT("/roles/:id", s.handleUpdateRoleConfig)
		adminGroup.DELETE("/roles/:id", s.handleDeleteRoleConfig)

		// 快捷回复词
		adminGroup.GET("/quick-reply", s.handleGetQuickReplyWords)
		adminGroup.POST("/quick-reply", s.handleCreateQuickReplyWord)
		adminGroup.PUT("/quick-reply/:id", s.handleUpdateQuickReplyWord)
		adminGroup.DELETE("/quick-reply/:id", s.handleDeleteQuickReplyWord)

		// 本地MCP功能
		adminGroup.GET("/mcp-functions", s.handleGetMCPFunctions)
		adminGroup.POST("/mcp-functions", s.handleCreateMCPFunction)
		adminGroup.PUT("/mcp-functions/:id", s.handleUpdateMCPFunction)
		adminGroup.DELETE("/mcp-functions/:id", s.handleDeleteMCPFunction)

		// 退出指令
		adminGroup.GET("/exit-commands", s.handleGetExitCommands)
		adminGroup.POST("/exit-commands", s.handleCreateExitCommand)
		adminGroup.PUT("/exit-commands/:id", s.handleUpdateExitCommand)
		adminGroup.DELETE("/exit-commands/:id", s.handleDeleteExitCommand)
	}
}

type ApplicationConfig struct {
	EnableMCPFilter bool `json:"enableMCPFilter"`
	QuickReply      bool `json:"quickReply"`
	SaveTtsAudio    bool `json:"saveTtsAudio"`
	SaveUserAudio   bool `json:"saveUserAudio"`
}

func (s *SystemConfigService) updateConfigAndRespond(
	c *gin.Context,
	update func(cfg *configs.Config) error,
	okMessage string,
	okData interface{},
) {
	if err := configs.UpdateCfgAndSaveToDB(database.GetServerConfigDB(), update); err != nil {
		s.logger.Error("保存配置失败: %v", err)
		httpStatus := 500
		message := "保存配置失败"
		var updateErr *configs.UpdateRejectedError
		if errors.As(err, &updateErr) {
			httpStatus = 400
			message = err.Error()
		}
		c.JSON(httpStatus, gin.H{
			"status":  "error",
			"message": message,
			"error":   err.Error(),
		})
		return
	}

	resp := gin.H{
		"status":  "ok",
		"message": okMessage,
	}
	if okData != nil {
		resp["data"] = okData
	}
	c.JSON(200, resp)
}

// 应用配置相关处理器
func (s *SystemConfigService) handleGetApplicationConfig(c *gin.Context) {
	cfg := configs.MustGetCfg()

	var config ApplicationConfig
	config.SaveTtsAudio = cfg.SaveTTSAudio
	config.SaveUserAudio = cfg.SaveUserAudio
	config.QuickReply = cfg.QuickReply

	c.JSON(200, gin.H{
		"status": "ok",
		"data":   config,
	})
}

func (s *SystemConfigService) handleUpdateApplicationConfig(c *gin.Context) {
	var config ApplicationConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "请求参数错误",
			"error":   err.Error(),
		})
		return
	}

	s.updateConfigAndRespond(c, func(cfg *configs.Config) error {
		cfg.SaveTTSAudio = config.SaveTtsAudio
		cfg.SaveUserAudio = config.SaveUserAudio
		cfg.QuickReply = config.QuickReply
		return nil
	}, "应用配置更新成功", config)
}

type AuthConfig struct {
	Token  string `json:"token"`
	Expiry int    `json:"expiry"`
}

// 认证配置相关处理器
func (s *SystemConfigService) handleGetAuthConfig(c *gin.Context) {
	cfg := configs.MustGetCfg()

	var config AuthConfig
	config.Token = cfg.Server.Token
	config.Expiry = cfg.Server.Auth.Store.Expiry

	c.JSON(200, gin.H{
		"status": "ok",
		"data":   config,
	})
}

func (s *SystemConfigService) handleUpdateAuthConfig(c *gin.Context) {
	var config AuthConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "请求参数错误",
			"error":   err.Error(),
		})
		return
	}
	s.updateConfigAndRespond(c, func(cfg *configs.Config) error {
		cfg.Server.Token = config.Token
		cfg.Server.Auth.Store.Expiry = config.Expiry
		return nil
	}, "认证配置更新成功", config)
}

type TransportConfig struct {
	Type    string          `json:"type"` // websocket/mqtt_udp
	Enabled bool            `json:"enabled"`
	Config  json.RawMessage `json:"config"` // arbitrary JSON payload
}

// 传输配置相关处理器
func (s *SystemConfigService) handleGetTransportConfig(c *gin.Context) {
	cfg := configs.MustGetCfg()

	var configsTransport []TransportConfig
	configsTransport = make([]TransportConfig, 0)
	str, _ := json.Marshal(cfg.Transport.WebSocket)
	configsTransport = append(configsTransport, TransportConfig{
		Type:    "websocket",
		Enabled: cfg.Transport.WebSocket.Enabled,
		Config:  str,
	})
	s.logger.Debug("获取 WebSocket 配置：%v", cfg.Transport.WebSocket.Enabled)

	strMqttUdp, _ := json.Marshal(cfg.Transport.MQTTUDP)
	configsTransport = append(configsTransport, TransportConfig{
		Type:    "mqtt_udp",
		Enabled: cfg.Transport.MQTTUDP.Enabled,
		Config:  strMqttUdp,
	})
	c.JSON(200, gin.H{
		"status": "ok",
		"data":   configsTransport,
	})
}

func (s *SystemConfigService) handleUpdateTransportConfig(c *gin.Context) {
	var configsTransport []TransportConfig
	if err := c.ShouldBindJSON(&configsTransport); err != nil {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "请求参数错误",
			"error":   err.Error(),
		})
		s.logger.Warn("绑定传输配置错误：%v", err)
		return
	}
	s.logger.Info("更新传输配置：%v", configsTransport)
	s.updateConfigAndRespond(c, func(cfg *configs.Config) error {
		for _, tcfg := range configsTransport {
			switch tcfg.Type {
			case "websocket":
				if err := json.Unmarshal(tcfg.Config, &cfg.Transport.WebSocket); err != nil {
					return err
				}
				cfg.Transport.WebSocket.Enabled = tcfg.Enabled
				s.logger.Debug("更新WebSocket配置：%v", cfg.Transport.WebSocket)
			case "mqtt_udp":
				if err := json.Unmarshal(tcfg.Config, &cfg.Transport.MQTTUDP); err != nil {
					return err
				}
				cfg.Transport.MQTTUDP.Enabled = tcfg.Enabled
			}
		}
		return nil
	}, "传输配置更新成功", configsTransport)
}

// WebConfig Web界面配置
type WebConfig struct {
	Enabled      bool   `json:"enabled"`
	Port         int    `json:"port"`
	StaticDir    string `json:"staticDir"`
	Websocket    string `json:"websocket"`
	VisionURL    string `json:"visionUrl"`
	ActivateText string `json:"activateText"` // 发送激活码时携带的文本
}

// LogConfig 日志配置
type LogConfig struct {
	LogLevel string `json:"logLevel"`
	LogDir   string `json:"logDir"`
	LogFile  string `json:"logFile"`
}

// RoleConfig 角色配置
type RoleConfig struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

// QuickReplyWord 快捷回复词
type QuickReplyWord struct {
	Word    string `json:"word"`
	Enabled bool   `json:"enabled"`
	Order   int    `json:"order"`
}

// LocalMCPFunction 本地MCP功能配置
type LocalMCPFunction struct {
	FunctionName string `json:"functionName"`
	Description  string `json:"description"`
	Enabled      bool   `json:"enabled"`
}

// ExitCommand 退出指令
type ExitCommand struct {
	Command string `json:"command"`
	Enabled bool   `json:"enabled"`
}

// Web配置相关处理器（使用 configs.GetCfg/UpdateCfgAndSaveToDB 读取/保存）
func (s *SystemConfigService) handleGetWebConfig(c *gin.Context) {
	cfg := configs.MustGetCfg()

	// 构造返回 DTO，从全局配置读取
	config := WebConfig{
		Port:         cfg.Web.Port,
		StaticDir:    cfg.Web.StaticDir,
		Websocket:    cfg.Web.Websocket,
		VisionURL:    cfg.Web.VisionURL,
		ActivateText: cfg.Web.ActivateText,
	}

	c.JSON(200, gin.H{
		"status": "ok",
		"data":   config,
	})
}

func (s *SystemConfigService) handleUpdateWebConfig(c *gin.Context) {
	var payload WebConfig
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "请求参数错误",
			"error":   err.Error(),
		})
		return
	}

	s.updateConfigAndRespond(c, func(cfg *configs.Config) error {
		cfg.Web.Port = payload.Port
		cfg.Web.StaticDir = payload.StaticDir
		cfg.Web.Websocket = payload.Websocket
		cfg.Web.VisionURL = payload.VisionURL
		cfg.Web.ActivateText = payload.ActivateText
		return nil
	}, "Web 配置更新成功", payload)
}

// 日志配置相关处理器
func (s *SystemConfigService) handleGetLogConfig(c *gin.Context) {
	cfg := configs.MustGetCfg()

	// 从全局配置读取并返回 DTO
	logCfg := LogConfig{
		LogLevel: cfg.Log.LogLevel,
		LogDir:   cfg.Log.LogDir,
		LogFile:  cfg.Log.LogFile,
	}

	c.JSON(200, gin.H{
		"status": "ok",
		"data":   logCfg,
	})
}

func (s *SystemConfigService) handleUpdateLogConfig(c *gin.Context) {
	var payload LogConfig
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "请求参数错误",
			"error":   err.Error(),
		})
		return
	}

	s.updateConfigAndRespond(c, func(cfg *configs.Config) error {
		cfg.Log.LogLevel = payload.LogLevel
		cfg.Log.LogDir = payload.LogDir
		cfg.Log.LogFile = payload.LogFile
		return nil
	}, "日志配置更新成功", payload)
}

// 角色配置相关处理器
func (s *SystemConfigService) handleGetRoleConfigs(c *gin.Context) {
	cfg := configs.MustGetCfg()

	var roles []RoleConfig
	for _, role := range cfg.Roles {
		roles = append(roles, RoleConfig{
			Name:        role.Name,
			Description: role.Description,
			Enabled:     role.Enabled,
		})
	}

	c.JSON(200, gin.H{
		"status": "ok",
		"data":   roles,
	})
}

func (s *SystemConfigService) handleCreateRoleConfig(c *gin.Context) {
	var config RoleConfig

	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "请求参数错误",
			"error":   err.Error(),
		})
		return
	}
	s.updateConfigAndRespond(c, func(cfg *configs.Config) error {
		cfg.Roles = append(cfg.Roles, configs.Role{
			Name:        config.Name,
			Description: config.Description,
			Enabled:     config.Enabled,
		})
		return nil
	}, "角色配置创建成功", config)
}

func (s *SystemConfigService) handleUpdateRoleConfig(c *gin.Context) {
	name := c.Param("id")

	var config RoleConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "请求参数错误",
			"error":   err.Error(),
		})
		return
	}

	s.updateConfigAndRespond(c, func(cfg *configs.Config) error {
		for i := range cfg.Roles {
			if cfg.Roles[i].Name == name {
				cfg.Roles[i].Description = config.Description
				cfg.Roles[i].Enabled = config.Enabled
				return nil
			}
		}
		return fmt.Errorf("角色不存在: %s", name)
	}, "角色配置更新成功", nil)
}

func (s *SystemConfigService) handleDeleteRoleConfig(c *gin.Context) {
	name := c.Param("id")
	s.updateConfigAndRespond(c, func(cfg *configs.Config) error {
		newRoles := make([]configs.Role, 0, len(cfg.Roles))
		for _, role := range cfg.Roles {
			if role.Name != name {
				newRoles = append(newRoles, role)
			}
		}
		cfg.Roles = newRoles
		return nil
	}, "角色配置删除成功", nil)
}

// 快捷回复词相关处理器
func (s *SystemConfigService) handleGetQuickReplyWords(c *gin.Context) {
	cfg := configs.MustGetCfg()

	var words []QuickReplyWord
	words = make([]QuickReplyWord, 0)
	for _, dbWord := range cfg.QuickReplyWords {
		words = append(words, QuickReplyWord{
			Word:    dbWord,
			Enabled: true,
		})
	}

	c.JSON(200, gin.H{
		"status": "ok",
		"data":   words,
	})
}

func (s *SystemConfigService) handleCreateQuickReplyWord(c *gin.Context) {
	var word QuickReplyWord
	if err := c.ShouldBindJSON(&word); err != nil {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "请求参数错误",
			"error":   err.Error(),
		})
		return
	}

	s.updateConfigAndRespond(c, func(cfg *configs.Config) error {
		cfg.QuickReplyWords = append(cfg.QuickReplyWords, word.Word)
		return nil
	}, "快捷回复词创建成功", word)
}

func (s *SystemConfigService) handleUpdateQuickReplyWord(c *gin.Context) {
	oldWord := c.Param("id")

	var word QuickReplyWord
	if err := c.ShouldBindJSON(&word); err != nil {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "请求参数错误",
			"error":   err.Error(),
		})
		return
	}

	s.updateConfigAndRespond(c, func(cfg *configs.Config) error {
		for i := range cfg.QuickReplyWords {
			if cfg.QuickReplyWords[i] == oldWord {
				cfg.QuickReplyWords[i] = word.Word
				return nil
			}
		}
		return fmt.Errorf("快捷回复词不存在: %s", oldWord)
	}, "快捷回复词更新成功", nil)
}

func (s *SystemConfigService) handleDeleteQuickReplyWord(c *gin.Context) {
	word := c.Param("id")
	s.updateConfigAndRespond(c, func(cfg *configs.Config) error {
		newWords := make([]string, 0, len(cfg.QuickReplyWords))
		for _, dbWord := range cfg.QuickReplyWords {
			if dbWord != word {
				newWords = append(newWords, dbWord)
			}
		}
		cfg.QuickReplyWords = newWords
		return nil
	}, "快捷回复词删除成功", nil)
}

// 本地MCP功能相关处理器
func (s *SystemConfigService) handleGetMCPFunctions(c *gin.Context) {
	cfg := configs.MustGetCfg()

	var functions []LocalMCPFunction
	functions = make([]LocalMCPFunction, 0)
	for _, dbFunc := range cfg.LocalMCPFun {
		functions = append(functions, LocalMCPFunction{
			FunctionName: dbFunc.Name,
			Description:  dbFunc.Description,
			Enabled:      dbFunc.Enabled,
		})
	}

	c.JSON(200, gin.H{
		"status": "ok",
		"data":   functions,
	})
}

func (s *SystemConfigService) handleCreateMCPFunction(c *gin.Context) {
	var function LocalMCPFunction
	if err := c.ShouldBindJSON(&function); err != nil {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "请求参数错误",
			"error":   err.Error(),
		})
		return
	}
	s.updateConfigAndRespond(c, func(cfg *configs.Config) error {
		cfg.LocalMCPFun = append(cfg.LocalMCPFun, configs.LocalMCPFun{
			Name:        function.FunctionName,
			Description: function.Description,
			Enabled:     function.Enabled,
		})
		return nil
	}, "MCP功能创建成功", function)
}

func (s *SystemConfigService) handleUpdateMCPFunction(c *gin.Context) {
	id := c.Param("id")
	var function LocalMCPFunction
	if err := c.ShouldBindJSON(&function); err != nil {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "请求参数错误",
			"error":   err.Error(),
		})
		return
	}

	s.updateConfigAndRespond(c, func(cfg *configs.Config) error {
		for i := range cfg.LocalMCPFun {
			if cfg.LocalMCPFun[i].Name == id {
				cfg.LocalMCPFun[i] = configs.LocalMCPFun{
					Name:        function.FunctionName,
					Description: function.Description,
					Enabled:     function.Enabled,
				}
				return nil
			}
		}
		return fmt.Errorf("MCP功能不存在: %s", id)
	}, "MCP功能更新成功", nil)
}

func (s *SystemConfigService) handleDeleteMCPFunction(c *gin.Context) {
	id := c.Param("id")
	s.updateConfigAndRespond(c, func(cfg *configs.Config) error {
		newFuncs := make([]configs.LocalMCPFun, 0, len(cfg.LocalMCPFun))
		for _, dbFunc := range cfg.LocalMCPFun {
			if dbFunc.Name != id {
				newFuncs = append(newFuncs, dbFunc)
			}
		}
		cfg.LocalMCPFun = newFuncs
		return nil
	}, "MCP功能删除成功", nil)
}

// 退出指令相关处理器
func (s *SystemConfigService) handleGetExitCommands(c *gin.Context) {
	cfg := configs.MustGetCfg()

	var commands []ExitCommand
	commands = make([]ExitCommand, 0)
	for _, dbCmd := range cfg.CMDExit {
		commands = append(commands, ExitCommand{
			Command: dbCmd,
			Enabled: true,
		})
	}

	c.JSON(200, gin.H{
		"status": "ok",
		"data":   commands,
	})
}

func (s *SystemConfigService) handleCreateExitCommand(c *gin.Context) {
	var command ExitCommand
	if err := c.ShouldBindJSON(&command); err != nil {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "请求参数错误",
			"error":   err.Error(),
		})
		return
	}

	s.updateConfigAndRespond(c, func(cfg *configs.Config) error {
		cfg.CMDExit = append(cfg.CMDExit, command.Command)
		return nil
	}, "退出指令创建成功", command)
}

func (s *SystemConfigService) handleUpdateExitCommand(c *gin.Context) {
	id := c.Param("id")
	var command ExitCommand
	if err := c.ShouldBindJSON(&command); err != nil {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "请求参数错误",
			"error":   err.Error(),
		})
		return
	}
	s.updateConfigAndRespond(c, func(cfg *configs.Config) error {
		for i := range cfg.CMDExit {
			if cfg.CMDExit[i] == id {
				cfg.CMDExit[i] = command.Command
				return nil
			}
		}
		return fmt.Errorf("退出指令不存在: %s", id)
	}, "退出指令更新成功", nil)
}

func (s *SystemConfigService) handleDeleteExitCommand(c *gin.Context) {
	id := c.Param("id")
	s.updateConfigAndRespond(c, func(cfg *configs.Config) error {
		newCmds := make([]string, 0, len(cfg.CMDExit))
		for _, dbCmd := range cfg.CMDExit {
			if dbCmd != id {
				newCmds = append(newCmds, dbCmd)
			}
		}
		cfg.CMDExit = newCmds
		return nil
	}, "退出指令删除成功", nil)
}
