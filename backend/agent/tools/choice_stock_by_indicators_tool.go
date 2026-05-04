package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"go-stock/backend/data"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/duke-git/lancet/v2/convertor"
	"github.com/duke-git/lancet/v2/random"
)

// @Author spark
// @Date 2025/8/5 11:17
// @Desc
//-----------------------------------------------------------------------------------

func GetChoiceStockByIndicatorsTool() tool.InvokableTool {
	return &ChoiceStockByIndicators{}
}

type ChoiceStockByIndicators struct {
}

func (c ChoiceStockByIndicators) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "ChoiceStockByIndicators",
		Desc: "根据自然语言筛选股票，返回股票筛选结果。支持股票代码/名称查询及技术指标分析。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"words": {
				Type: "string",
				Desc: "选股条件或股票代码/名称。" +
					"例：上海贝岭,MACD,KDJ,RSI,BOLL,5日均线,成交量" +
					"例：创新药,半导体;PE<30;净利润增长率>50%",
				Required: true,
			},
		}),
	}, nil
}

func (c ChoiceStockByIndicators) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	parms := map[string]any{}
	err := json.Unmarshal([]byte(argumentsInJSON), &parms)
	if err != nil {
		return "", err
	}
	content := "无符合条件的数据"
	words := parms["words"].(string)
	res := data.NewSearchStockApi(words).SearchStock(random.RandInt(5, 20))
	if convertor.ToString(res["code"]) == "100" {
		resData := res["data"].(map[string]any)
		result := resData["result"].(map[string]any)
		dataList := result["dataList"].([]any)
		columns := result["columns"].([]any)
		headers := map[string]string{}
		for _, v := range columns {
			//logger.SugaredLogger.Infof("v:%+v", v)
			d := v.(map[string]any)
			//logger.SugaredLogger.Infof("key:%s title:%s dateMsg:%s unit:%s", d["key"], d["title"], d["dateMsg"], d["unit"])
			title := convertor.ToString(d["title"])
			if convertor.ToString(d["dateMsg"]) != "" {
				title = title + "[" + convertor.ToString(d["dateMsg"]) + "]"
			}
			if convertor.ToString(d["unit"]) != "" {
				title = title + "(" + convertor.ToString(d["unit"]) + ")"
			}
			headers[d["key"].(string)] = title
		}
		table := &[]map[string]any{}
		for _, v := range dataList {
			d := v.(map[string]any)
			tmp := map[string]any{}
			for key, title := range headers {
				tmp[title] = convertor.ToString(d[key])
			}
			*table = append(*table, tmp)
		}
		jsonData, _ := json.Marshal(*table)
		markdownTable, _ := JSONToMarkdownTable(jsonData)
		//logger.SugaredLogger.Infof("markdownTable=\n%s", markdownTable)
		content = "\r\n### 工具筛选出的股票数据：\r\n" + markdownTable + "\r\n"
	}
	return content, nil
}

// JSONToMarkdownTable 将JSON数据转换为Markdown表格
func JSONToMarkdownTable(jsonData []byte) (string, error) {
	var data []map[string]interface{}
	err := json.Unmarshal(jsonData, &data)
	if err != nil {
		return "", err
	}

	if len(data) == 0 {
		return "", nil
	}

	// 获取表头
	headers := []string{}
	for key := range data[0] {
		headers = append(headers, key)
	}

	// 构建表头行
	headerRow := "|"
	for _, header := range headers {
		headerRow += fmt.Sprintf(" %s |", header)
	}
	headerRow += "\n"

	// 构建分隔行
	separatorRow := "|"
	for range headers {
		separatorRow += " --- |"
	}
	separatorRow += "\n"

	// 构建数据行
	bodyRows := ""
	for _, rowData := range data {
		bodyRow := "|"
		for _, header := range headers {
			value := rowData[header]
			bodyRow += fmt.Sprintf(" %v |", value)
		}
		bodyRows += bodyRow + "\n"
	}

	return headerRow + separatorRow + bodyRows, nil
}
