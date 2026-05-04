//go:build darwin
// +build darwin

package data

import (
	"fmt"
	"go-stock/backend/logger"
	"os/exec"
	"regexp"
	"strings"
)

// sanitizeInput 验证和清理用户输入，防止命令注入
func sanitizeInput(input string) string {
	if input == "" {
		return ""
	}
	
	// 移除潜在危险字符，只保留字母数字和常见标点符号
	reg := regexp.MustCompile(`[^\w\s\u4e00-\u9fff.,!?;:'"-]`)
	cleaned := reg.ReplaceAllString(input, "")
	
	// 限制最大长度
	if len(cleaned) > 200 {
		cleaned = cleaned[:200]
	}
	
	// 转义引号和其他特殊字符
	cleaned = strings.ReplaceAll(cleaned, `"`, `\"`)
	cleaned = strings.ReplaceAll(cleaned, `'`, `\'`)
	cleaned = strings.ReplaceAll(cleaned, `\`, `\\`)
	
	return cleaned
}

// AlertWindowsApi @Author 2lovecode
// @Date 2025/02/06 17:50
// @Desc
// -----------------------------------------------------------------------------------
type AlertWindowsApi struct {
	AppID string
	// 窗口标题
	Title string
	// 窗口内容
	Content string
	// 窗口图标
	Icon string
}

func NewAlertWindowsApi(AppID string, Title string, Content string, Icon string) *AlertWindowsApi {
	return &AlertWindowsApi{
		AppID:   AppID,
		Title:   Title,
		Content: Content,
		Icon:    Icon,
	}
}

func (a AlertWindowsApi) SendNotification() bool {
	if GetSettingConfig().LocalPushEnable == false {
		logger.SugaredLogger.Error("本地推送未开启")
		return false
	}

	// 对用户输入进行验证和转义，防止命令注入
	content := sanitizeInput(a.Content)
	title := sanitizeInput(a.Title)
	
	if content == "" || title == "" {
		logger.SugaredLogger.Error("通知内容或标题无效")
		return false
	}

	script := fmt.Sprintf(`display notification "%s" with title "%s"`, content, title)

	cmd := exec.Command("osascript", "-e", script)
	err := cmd.Run()
	if err != nil {
		logger.SugaredLogger.Error(err)
		return false
	}
	return true
}