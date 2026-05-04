package agent

import (
	"context"
	"fmt"
	"go-stock/backend/data"
	"go-stock/backend/logger"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
)

// IndustryChainInput 产业链分析所需输入数据。
type IndustryChainInput struct {
	StockCode     string   // 股票代码（ts_code 格式，如 600519.SH）
	StockName     string   // 股票名称
	Industry      string   // 所处行业
	BoardTags     []string // 所属板块
	ConceptTags   []string // 关联概念
	BusinessDesc  string   // 核心产品/主营业务描述
	FinanceSummary string  // 财务摘要（可选）
}

// GenerateIndustryChainReport 使用 AI 生成个股产业链深度分析报告。
// 若 AI 调用失败，返回空字符串和 error。
func GenerateIndustryChainReport(ctx context.Context, input IndustryChainInput) (string, error) {
	config := data.GetSettingConfig()
	if config == nil || len(config.AiConfigs) == 0 {
		return "", fmt.Errorf("未配置 AI 模型")
	}

	aiConfig := config.AiConfigs[0]
	for _, c := range config.AiConfigs {
		if strings.TrimSpace(c.ApiKey) != "" {
			aiConfig = c
			break
		}
	}
	if strings.TrimSpace(aiConfig.ApiKey) == "" {
		return "", fmt.Errorf("未找到可用的 AI 配置（API Key 为空）")
	}

	aiConfig.Thinking = false
	aiConfig.MaxTokens = max(aiConfig.MaxTokens, 4096)

	chatModel, err := CreateChatModel(ctx, *aiConfig)
	if err != nil {
		return "", fmt.Errorf("创建 AI 模型失败: %w", err)
	}

	displayName := input.StockName
	if displayName == "" {
		displayName = input.StockCode
	}

	systemPrompt := `你是一位拥有20年经验的资深产业研究分析师，精通产业链深度研究、竞争格局分析和投资价值评估。

请根据提供的股票基本信息，生成一份专业、详尽、图文并茂的【个股产业链深度分析报告】。

## 报告结构要求（严格按此顺序输出）

### 一、公司定位与业务概览
用 **3-5 句精炼概括** 行业地位与核心竞争力，然后以 **Markdown 表格** 展示主营业务拆解：

| 业务板块 | 产品/服务 | 收入占比(估) | 毛利率水平 | 行业排名 |
|---------|----------|-------------|-----------|---------|
| ... | ... | ... | ... | ... |

最后用 1-2 句判断当前行业赛道的景气度（景气上行/平稳/承压）。

### 二、产业链全景图
**必须** 使用 Mermaid flowchart 绘制产业链结构图，从上游到下游完整展示：

` + "```mermaid" + `
flowchart LR
    A[上游：原材料/核心部件] --> B[中游：组件制造/系统集成]
    B --> C[下游：终端应用/销售服务]
    C --> D[终端用户/客户]
` + "```" + `

然后用文字详细描述产业链各环节的价值传递关系，并用表格汇总各环节核心价值活动：

| 产业链环节 | 核心价值活动 | 进入壁垒 | 毛利率区间 | 代表企业 |
|-----------|-------------|---------|-----------|---------|
| ... | ... | ... | ... | ... |

**关键：将标的公司所在的环节用 **粗体** 或 🔵 特别标注。**

### 三、上游核心公司分析
用 **Markdown 表格** 列出至少 4-6 家有代表性的上游公司：

| 序号 | 公司名称 | 股票代码 | 供应产品/服务 | 供应占比(估) | 可替代性 | 近期动态 |
|-----|---------|---------|-------------|-------------|---------|---------|
| 1 | ... | xxxXXX | ... | 高/中/低 | 低/中/高 | ... |

表格后，用文字深入分析 **上游集中度、议价能力、供应风险**（2-3段）。

### 四、下游核心公司分析
用 **Markdown 表格** 列出至少 4-6 家有代表性的下游公司/客户群：

| 序号 | 公司名称 | 股票代码 | 采购产品/服务 | 需求景气度 | 增长趋势 | 近期动态 |
|-----|---------|---------|-------------|-----------|---------|---------|
| 1 | ... | xxxXXX | ... | 高/中/低 | ↑/→/↓ | ... |

表格后，用文字深入分析 **下游需求结构、客户黏性、市场扩张空间**（2-3段）。

### 五、同业竞争格局对比
用 **Markdown 表格** 进行关键指标横向对比（至少 4-6 家竞争对手）：

| 公司名称 | 股票代码 | 市值(估) | 营收规模 | 净利率 | 核心优势 | 竞争策略 |
|---------|---------|---------|---------|-------|---------|---------|
| **标的公司** | ... | ... | ... | ... | ... | ... |
| 竞对1 | ... | ... | ... | ... | ... | ... |

然后总结竞争格局要点（2-3段文字）。

### 六、产业链投资价值分析
从以下维度进行结构化分析，**每个维度用单独的表格呈现**：

**政策维度：**
| 政策方向 | 影响环节 | 力度评估 | 时间窗口 |
|---------|---------|---------|---------|
| ... | ... | 强/中/弱 | 短期/中期/长期 |

**技术维度：**
| 技术趋势 | 影响环节 | 成熟度 | 标的受益程度 |
|---------|---------|-------|------------|
| ... | ... | 成熟/成长/萌芽 | 高/中/低 |

**需求维度：**
| 下游需求 | 景气判断 | 增长驱动因素 | 标的受益程度 |
|---------|---------|------------|------------|
| ... | ↑/→/↓ | ... | 高/中/低 |

接着用 2-3 段文字分析产业链各环节投资价值排序、标的议价能力、潜在整合机会。

### 七、风险提示
用 **表格 + 文字** 呈现：

| 风险类别 | 具体风险 | 影响程度 | 应对措施 |
|---------|---------|---------|---------|
| 上游供应 | ... | 高/中/低 | ... |
| 下游需求 | ... | 高/中/低 | ... |
| 行业竞争 | ... | 高/中/低 | ... |
| 政策监管 | ... | 高/中/低 | ... |

## 输出格式要求
- 全文使用 Markdown，标题层级清晰（## → ###）
- **所有上市公司必须标注股票代码**（如 600519.SH / 000858.SZ）
- 表格列对齐整齐，包含数据处用估算值并标注"(估)"
- Mermaid 图表放在 ` + "```mermaid" + ` 代码块中（必须渲染为图表）
- 关键结论用 **粗体** 标注
- 数据引用处注明来源（"据公开财报/行业数据"）
- 避免使用"建议买入/卖出"等违规表述，使用"值得关注""需警惕"等中性措辞`

	userPrompt := fmt.Sprintf("## 股票基本信息\n\n- **股票名称**：%s\n- **股票代码**：`%s`\n- **所处行业**：%s\n- **所属板块**：%s\n- **关联概念**：%s\n\n### 核心业务描述\n\n%s",
		displayName,
		input.StockCode,
		input.Industry,
		joinTags(input.BoardTags),
		joinTags(input.ConceptTags),
		input.BusinessDesc,
	)

	if input.FinanceSummary != "" {
		userPrompt += fmt.Sprintf("\n\n### 财务摘要\n\n%s", input.FinanceSummary)
	}

	messages := []*schema.Message{
		{Role: schema.System, Content: systemPrompt},
		{Role: schema.User, Content: userPrompt},
	}

	logger.SugaredLogger.Infof("IndustryChain: generating report for %s (%s) model=%s",
		input.StockCode, input.StockName, aiConfig.ModelName)

	startTime := time.Now()
	resp, err := chatModel.Generate(ctx, messages)
	elapsed := time.Since(startTime)

	if err != nil {
		logger.SugaredLogger.Errorf("IndustryChain: AI generate failed for %s after %v: %v",
			input.StockCode, elapsed, err)
		return "", fmt.Errorf("AI 生成报告失败: %w", err)
	}
	if resp == nil || resp.Content == "" {
		return "", fmt.Errorf("AI 返回空内容")
	}

	logger.SugaredLogger.Infof("IndustryChain: report generated for %s in %v, length=%d chars",
		input.StockCode, elapsed, len(resp.Content))

	return resp.Content, nil
}

func joinTags(tags []string) string {
	if len(tags) == 0 {
		return "未识别"
	}
	return strings.Join(tags, "、")
}
