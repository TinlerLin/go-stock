package server

import (
	"context"
	"embed"
	"encoding/json"
	"go-stock/backend/data"
	"go-stock/backend/db"
	"go-stock/backend/logger"
	"go-stock/backend/models"
	"io/fs"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

//go:embed static
var staticFS embed.FS

type app struct{}

type aiConfigResp struct {
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	BaseURL   string `json:"baseUrl"`
	ModelName string `json:"modelName"`
}

type chatRequest struct {
	Question    string `json:"question"`
	AIConfigID  int    `json:"aiConfigId"`
	SysPromptID int    `json:"sysPromptId"`
	Thinking    bool   `json:"thinking"`
	EnableTools bool   `json:"enableTools"`
	HistoryJSON string `json:"historyJSON"`
}

type sessionSaveRequest struct {
	SessionId string                      `json:"sessionId"`
	Messages  []models.AiAssistantMessage `json:"messages"`
}

type shareRequest struct {
	Text  string `json:"text"`
	Title string `json:"title"`
}

// Start 在当前进程内启动 ai-assistant-web 服务（阻塞，适合放在 goroutine 中）。
func Start() error {
	checkDir("data")
	checkDir("logs")

	// 当作为 go-stock 子组件启动时，db 可能已经初始化过。
	if db.Dao == nil {
		db.Init("")
	}
	autoMigrate()
	data.InitAnalyzeSentiment()

	a := &app{}
	mux := http.NewServeMux()

	mux.HandleFunc("/api/health", a.health)
	mux.HandleFunc("/api/vip-status", a.vipStatus)
	mux.HandleFunc("/api/ai-configs", a.getAIConfigs)
	mux.HandleFunc("/api/prompts", a.getPrompts)
	mux.HandleFunc("/api/session", a.session)
	mux.HandleFunc("/api/chat/summary-stream", a.summaryChatStream)
	mux.HandleFunc("/api/share", a.shareText)

	subFS, err := fs.Sub(staticFS, "static")
	if err != nil {
		logger.SugaredLogger.Fatalf("load static files failed: %v", err)
	}
	staticServer := http.FileServer(http.FS(subFS))
	mux.Handle("/", staticServer)

	addr := getAddr()
	logger.SugaredLogger.Infof("ai-assistant-web started at: %s", addr)
	return http.ListenAndServe(addr, withCORS(mux))
}

func (a *app) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"time": time.Now().Format("2006-01-02 15:04:05"),
	})
}

func (a *app) vipStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	level, active := data.EffectiveSponsorVipLevel()
	ok := active && level >= 2
	payload := map[string]any{
		"ok":       ok,
		"vipLevel": level,
		"active":   active,
	}
	if !ok {
		payload["message"] = vipDeniedMessage(level, active)
	}
	writeJSON(w, http.StatusOK, payload)
}

func vipDeniedMessage(level int, active bool) string {
	if !active && level > 0 {
		return "检测到赞助信息，但当前不在 VIP 有效期内或尚未到授权生效时间。请在 go-stock 客户端「关于」确认赞助状态。"
	}
	return "go-stock AI 助手（Web）仅对 VIP2 及以上有效赞助用户开放。请在 go-stock 桌面客户端「关于」页面填写赞助码后，使用与本机相同的 data 目录启动服务。"
}

func requireVip2(w http.ResponseWriter) bool {
	level, active := data.EffectiveSponsorVipLevel()
	if active && level >= 2 {
		return true
	}
	writeJSON(w, http.StatusForbidden, map[string]any{
		"code":     "VIP2_REQUIRED",
		"message":  vipDeniedMessage(level, active),
		"vipLevel": level,
		"active":   active,
	})
	return false
}

func (a *app) getAIConfigs(w http.ResponseWriter, _ *http.Request) {
	if !requireVip2(w) {
		return
	}
	cfgs := data.GetSettingConfig().AiConfigs
	resp := make([]aiConfigResp, 0, len(cfgs))
	for _, c := range cfgs {
		resp = append(resp, aiConfigResp{
			ID:        c.ID,
			Name:      c.Name,
			BaseURL:   c.BaseUrl,
			ModelName: c.ModelName,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *app) getPrompts(w http.ResponseWriter, r *http.Request) {
	if !requireVip2(w) {
		return
	}
	
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	promptType := strings.TrimSpace(r.URL.Query().Get("type"))
	
	// 验证输入参数，防止路径遍历或其他非法字符
	if !isValidParam(name) || !isValidParam(promptType) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid parameter"})
		return
	}
	
	res := data.NewPromptTemplateApi().GetPromptTemplates(name, promptType)
	writeJSON(w, http.StatusOK, res)
}

func (a *app) session(w http.ResponseWriter, r *http.Request) {
	if !requireVip2(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		sessionId := r.URL.Query().Get("sessionId")
		
		// 验证 sessionId 是否符合预期格式
		if sessionId != "" && !isValidSessionId(sessionId) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid session id"})
			return
		}
		
		res, err := data.GetAiAssistantSession(sessionId)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, res)
	case http.MethodPost:
		var req sessionSaveRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		
		// 验证 session id
		if !isValidSessionId(req.SessionId) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid session id"})
			return
		}
		
		if err := data.SaveAiAssistantSession(req.SessionId, req.Messages); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *app) summaryChatStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !requireVip2(w) {
		return
	}

	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	req.Question = strings.TrimSpace(req.Question)
	if req.Question == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "question is required"})
		return
	}

	if req.AIConfigID <= 0 {
		cfgs := data.GetSettingConfig().AiConfigs
		if len(cfgs) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no ai config found"})
			return
		}
		req.AIConfigID = int(cfgs[0].ID)
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "stream not supported"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	var sysPromptID *int
	if req.SysPromptID > 0 {
		sysPromptID = &req.SysPromptID
	}

	history := parseHistory(req.HistoryJSON)
	tools := make([]data.Tool, 0)
	if req.EnableTools {
		tools = data.Tools(tools)
	}
	o := data.NewDeepSeekOpenAi(ctx, req.AIConfigID)
	var ch <-chan map[string]any
	if req.EnableTools {
		ch = o.NewSummaryStockNewsStreamWithTools(req.Question, sysPromptID, tools, req.Thinking, history)
	} else {
		ch = o.NewSummaryStockNewsStream(req.Question, sysPromptID, req.Thinking, history)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				_, _ = w.Write([]byte("event: done\ndata: [DONE]\n\n"))
				flusher.Flush()
				return
			}
			raw, err := json.Marshal(msg)
			if err != nil {
				continue
			}
			_, _ = w.Write([]byte("data: " + string(raw) + "\n\n"))
			flusher.Flush()
		}
	}
}

func (a *app) shareText(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !requireVip2(w) {
		return
	}
	var req shareRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	req.Text = strings.TrimSpace(req.Text)
	req.Title = strings.TrimSpace(req.Title)
	if req.Text == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "内容为空"})
		return
	}
	if req.Title == "" {
		req.Title = "AI助手"
	}
	analysisTime := time.Now().Format("2006/01/02")
	resp, err := resty.New().SetHeader("ua-x", "go-stock").R().SetFormData(map[string]string{
		"text":         req.Text,
		"stockCode":    req.Title,
		"stockName":    req.Title,
		"analysisTime": analysisTime,
	}).Post("https://go-stock.sparkmemory.top:16688/upload")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": resp.String()})
}

func parseHistory(historyJSON string) []map[string]interface{} {
	historyJSON = strings.TrimSpace(historyJSON)
	if historyJSON == "" {
		return nil
	}
	var list []models.AiAssistantMessage
	if err := json.Unmarshal([]byte(historyJSON), &list); err != nil {
		return nil
	}
	history := make([]map[string]interface{}, 0, len(list))
	for _, m := range list {
		item := map[string]interface{}{
			"role":    m.Role,
			"content": m.Content,
		}
		if m.Reasoning != "" {
			item["reasoning_content"] = m.Reasoning
		}
		history = append(history, item)
	}
	return history
}

func (a *app) chatStream(w http.ResponseWriter, r *http.Request) {
	// 保留旧接口兼容，转发到新的 summary 接口能力
	a.summaryChatStream(w, r)
}

func autoMigrate() {
	db.Dao.AutoMigrate(&data.Settings{})
	db.Dao.AutoMigrate(&data.AIConfig{})
	db.Dao.AutoMigrate(&models.PromptTemplate{})
	db.Dao.AutoMigrate(&models.AiAssistantSession{})
}

func getAddr() string {
	addr := strings.TrimSpace(os.Getenv("AI_ASSISTANT_WEB_ADDR"))
	if addr == "" {
		addr = ":18888"
	}
	return addr
}

func checkDir(dir string) {
	_, err := os.Stat(dir)
	if os.IsNotExist(err) {
		_ = os.Mkdir(dir, 0700)
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 从环境变量获取允许的源，如果没有则默认为开发环境的地址
		allowedOrigin := os.Getenv("AI_ASSISTANT_ALLOWED_ORIGIN")
		if allowedOrigin == "" {
			allowedOrigin = "http://localhost:5173"
		}
		
		// 验证请求来源是否在允许列表中
		origin := r.Header.Get("Origin")
		if origin != "" {
			if isValidOrigin(origin, allowedOrigin) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			}
		} else {
			// 如果没有 Origin 头，设置默认值
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		}
		
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isValidOrigin 验证请求来源是否合法
func isValidOrigin(requestedOrigin, allowedOrigin string) bool {
	// 支持多个允许的源，用逗号分隔
	allowedOrigins := strings.Split(allowedOrigin, ",")
	for _, origin := range allowedOrigins {
		origin = strings.TrimSpace(origin)
		if origin == "*" || requestedOrigin == origin {
			return true
		}
	}
	return false
}

// isValidParam 验证参数是否合法
func isValidParam(param string) bool {
	if param == "" {
		return true // 空值是允许的
	}
	
	// 检查长度限制
	if len(param) > 100 {
		return false
	}
	
	// 验证只包含字母数字和基本标点符号
	validRegex := regexp.MustCompile(`^[a-zA-Z0-9\u4e00-\u9fa5 _\-.,!?;:]*$`)
	return validRegex.MatchString(param)
}

// isValidSessionId 验证session id是否合法
func isValidSessionId(sessionId string) bool {
	if sessionId == "" {
		return true // 空值是允许的
	}
	
	// 验证sessionId格式，例如UUID格式或数字
	uuidRegex := regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)
	numRegex := regexp.MustCompile(`^\d+$`)
	
	return uuidRegex.MatchString(sessionId) || numRegex.MatchString(sessionId)
}
