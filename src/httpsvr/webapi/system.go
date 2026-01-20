package webapi

import (
	"context"
	"encoding/json"
	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/configs/database"
	"xiaozhi-server-go/src/core/utils"

	"github.com/gin-gonic/gin"
)

type DefaultAdminService struct {
	logger *utils.Logger
	config *configs.Config
}

// NewDefaultAdminService 构造函数
func NewDefaultAdminService(
	config *configs.Config,
	logger *utils.Logger,
) (*DefaultAdminService, error) {
	service := &DefaultAdminService{
		logger: logger,
		config: config,
	}

	return service, nil
}

// Start 实现 AdminService 接口，注册所有 Admin 相关路由
func (s *DefaultAdminService) Start(
	ctx context.Context,
	engine *gin.Engine,
	apiGroup *gin.RouterGroup,
) error {
	apiGroup.GET("/admin", s.handleGet)

	// 需要登录和管理员权限的分组
	adminGroup := apiGroup.Group("")
	// 查看模型不需要管理员权限
	adminGroup.Use(AuthMiddleware())
	{
		adminGroup.GET("/admin/system", s.handleSystemGet)
		adminGroup.GET("/admin/system/providers/:type", s.handleSystemProvidersType)
	}
	adminGroup.Use(AuthMiddleware(), AdminMiddleware())
	{
		adminGroup.POST("/admin/system", s.handleSystemPost)

		adminGroup.DELETE("/admin/system/device", s.handleDeviceDeleteAdmin)
		// providers
		adminGroup.GET("/admin/system/providers", s.handleSystemProvidersGet)
		adminGroup.GET("/admin/system/providers/:type/:name", s.handleSystemProvidersGetByName)

		adminGroup.POST("/admin/system/providers/create", s.handleSystemProvidersCreate)
		adminGroup.PUT("/admin/system/providers/:type/:name", s.handleSystemProvidersUpdate)
		adminGroup.DELETE("/admin/system/providers/:type/:name", s.handleSystemProvidersDelete)
	}

	s.logger.Info("Admin HTTP服务路由注册完成")
	return nil
}

func (s *DefaultAdminService) handleGet(c *gin.Context) {
	c.JSON(200, gin.H{
		"status":  "ok",
		"message": "Admin service is running",
	})
}

type SystemConfig struct {
	SelectedASR     string   `                  json:"selectedASR"`
	SelectedTTS     string   `                  json:"selectedTTS"`
	SelectedLLM     string   `                  json:"selectedLLM"`
	SelectedVLLLM   string   `                  json:"selectedVLLLM"`
	Prompt          string   ` json:"prompt"`
	QuickReplyWords []string `                  json:"quickReplyWords"` // 存储为 JSON 数组
}

// handleSystemGet 获取系统配置
// @Summary 获取系统配置
// @Description 获取当前系统配置
// @Tags System
// @Produce json
// @Success 200 {object} map[string]interface{} "系统配置信息"
// @Router /admin/system [get]
func (s *DefaultAdminService) handleSystemGet(c *gin.Context) {
	cfg := configs.MustGetCfg()

	var config SystemConfig
	selected := cfg.SelectedModule
	if selected == nil {
		selected = map[string]string{}
	}

	config.SelectedASR = selected["ASR"]
	config.SelectedTTS = selected["TTS"]
	config.SelectedLLM = selected["LLM"]
	config.SelectedVLLLM = selected["VLLLM"]
	config.Prompt = cfg.DefaultPrompt
	config.QuickReplyWords = cfg.QuickReplyWords

	var data map[string]interface{}
	tmp, _ := json.Marshal(config)
	json.Unmarshal(tmp, &data)

	asrList, ttsList, llmList, vllmList := database.GetProviderNameList(database.AdminUserID)
	data["asrList"] = asrList
	data["llmList"] = llmList
	data["ttsList"] = ttsList
	data["vllmList"] = vllmList

	// fmt.Println("JSON字符串:", data)
	c.JSON(200, gin.H{
		"status":  "ok",
		"message": "System configuration retrieved successfully",
		"data":    data,
	})
}

// handleSystemGet 获取系统配置
// @Summary 获取系统配置
// @Description 获取当前系统配置
// @Tags System
// @Produce json
// @Success 200 {object} map[string]interface{} "系统配置信息"
// @Router /admin/system [get]
func (s *DefaultAdminService) handleSystemPost(c *gin.Context) {
	// 定义请求结构体
	var requestData struct {
		Data string `json:"data"`
	}

	// 绑定JSON数据到结构体
	if err := c.ShouldBindJSON(&requestData); err != nil {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "Invalid JSON format",
			"error":   err.Error(),
		})
		return
	}

	// 检查data字段是否为空
	if requestData.Data == "" {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "Missing 'data' field in request body",
			"error":   "data field is required",
		})
		return
	}

	s.logger.Info("Received system configuration data: %s", requestData.Data)

	// 解析data字段中的JSON字符串到SystemConfig结构体
	var config SystemConfig
	if err := json.Unmarshal([]byte(requestData.Data), &config); err != nil {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "Invalid system configuration data",
			"error":   err.Error(),
		})
		return
	}

	if err := configs.UpdateCfgAndSaveToDB(database.GetServerConfigDB(), func(cfg *configs.Config) error {
		if cfg.SelectedModule == nil {
			cfg.SelectedModule = make(map[string]string)
		}
		cfg.SelectedModule["ASR"] = config.SelectedASR
		cfg.SelectedModule["TTS"] = config.SelectedTTS
		cfg.SelectedModule["LLM"] = config.SelectedLLM
		cfg.SelectedModule["VLLLM"] = config.SelectedVLLLM
		cfg.DefaultPrompt = config.Prompt
		cfg.QuickReplyWords = config.QuickReplyWords
		return nil
	}); err != nil {
		s.logger.Error("保存系统配置失败: %v", err)
		c.JSON(500, gin.H{
			"status":  "error",
			"message": "System configuration save failed",
		})
		return
	}

	c.JSON(200, gin.H{
		"status":  "ok",
		"message": "System configuration saved successfully",
	})
}
