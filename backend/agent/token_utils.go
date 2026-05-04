package agent

import (
	"go-stock/backend/logger"
	"strings"
	"unicode"

	"github.com/cloudwego/eino/schema"
)

const (
	chineseCharsPerToken = 1.5
	englishCharsPerToken = 4.0
	// 以下预留用于 estimateMessagesTokens 之外的系统/工具/ReAct 开销，过大会过早压缩用户上下文。
	// PlanExecute 中「已完成步骤」另见 agent.go 的 compressExecutedStepResult。
	toolsTokenReserve  = 8000
	skillPromptReserve = 4000
	reactLoopReserve   = 8000
	safetyMargin       = 0.85
)

func estimateTokens(text string) int {
	if text == "" {
		return 0
	}
	chineseCount := 0
	nonChineseCount := 0
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			chineseCount++
		} else {
			nonChineseCount++
		}
	}
	tokens := float64(chineseCount)/chineseCharsPerToken + float64(nonChineseCount)/englishCharsPerToken
	return int(tokens) + 1
}

func estimateMessagesTokens(messages []*schema.Message) int {
	total := 0
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		total += estimateTokens(msg.Content)
		for _, tc := range msg.ToolCalls {
			total += estimateTokens(tc.Function.Name)
			total += estimateTokens(tc.Function.Arguments)
		}
		total += 4
	}
	return total
}

func getMaxInputTokens(maxTokens int, toolCount int) int {
	if maxTokens <= 0 {
		return 120000
	}
	availableTokens := int(float64(maxTokens) * safetyMargin)
	toolReserve := estimateToolTokens(toolCount)
	reserved := toolReserve + skillPromptReserve + reactLoopReserve
	result := availableTokens - reserved
	if result < 4000 {
		result = 4000
	}
	return result
}

func estimateToolTokens(toolCount int) int {
	if toolCount <= 0 {
		return toolsTokenReserve
	}
	estimated := toolCount * 200
	if estimated < 2000 {
		estimated = 2000
	}
	if estimated > toolsTokenReserve {
		estimated = toolsTokenReserve
	}
	return estimated
}

func trimHistoryMessages(historyMessages []*schema.Message, maxTokens int) []*schema.Message {
	if len(historyMessages) == 0 {
		return historyMessages
	}

	currentTokens := estimateMessagesTokens(historyMessages)
	if currentTokens <= maxTokens {
		return historyMessages
	}

	halfLen := len(historyMessages) / 2
	if halfLen > 0 {
		trimmed := historyMessages[halfLen:]
		trimmedTokens := estimateMessagesTokens(trimmed)
		if trimmedTokens <= maxTokens {
			return trimmed
		}
	}

	result := []*schema.Message{}
	tokenSum := 0
	for i := len(historyMessages) - 1; i >= 0; i-- {
		msgTokens := estimateTokens(historyMessages[i].Content) + 4
		if tokenSum+msgTokens > maxTokens {
			break
		}
		tokenSum += msgTokens
		result = append([]*schema.Message{historyMessages[i]}, result...)
	}

	if len(result) == 0 && len(historyMessages) > 0 {
		lastMsg := historyMessages[len(historyMessages)-1]
		content := lastMsg.Content
		maxChars := maxTokens * 2
		if len(content) > maxChars {
			lastMsg = &schema.Message{
				Role:    lastMsg.Role,
				Content: content[len(content)-maxChars:] + "\n...(更早的内容已省略)",
			}
		}
		result = []*schema.Message{lastMsg}
	}

	return result
}

func trimToolResult(content string, maxTokens int) string {
	if content == "" {
		return content
	}
	estimatedTokens := estimateTokens(content)
	if estimatedTokens <= maxTokens {
		return content
	}
	maxChars := int(float64(maxTokens) * englishCharsPerToken * 0.8)
	if maxChars > len(content) {
		maxChars = len(content)
	}

	lines := strings.Split(content, "\n")
	var result []string
	charCount := 0
	for i := len(lines) - 1; i >= 0; i-- {
		lineLen := len(lines[i]) + 1
		if charCount+lineLen > maxChars {
			break
		}
		charCount += lineLen
		result = append([]string{lines[i]}, result...)
	}

	if len(result) == 0 && len(lines) > 0 {
		lastLine := lines[len(lines)-1]
		if len(lastLine) > maxChars {
			lastLine = lastLine[len(lastLine)-maxChars:]
		}
		result = []string{lastLine}
	}

	return strings.Join(result, "\n") + "\n\n...(内容过长，已截断显示)"
}

func compressMessages(messages []*schema.Message, maxTokens int) []*schema.Message {
	if len(messages) == 0 {
		return messages
	}

	currentTokens := estimateMessagesTokens(messages)
	if currentTokens <= maxTokens {
		return messages
	}

	logger.SugaredLogger.Infof("MessageRewriter: 上下文压缩前 %d 条消息, 估算 %d tokens, 预算 %d tokens",
		len(messages), currentTokens, maxTokens)

	result := make([]*schema.Message, 0, len(messages))

	var systemMsgs []*schema.Message
	var nonSystemMsgs []*schema.Message
	for _, msg := range messages {
		if msg.Role == schema.System {
			systemMsgs = append(systemMsgs, msg)
		} else {
			nonSystemMsgs = append(nonSystemMsgs, msg)
		}
	}

	result = append(result, systemMsgs...)
	systemTokens := estimateMessagesTokens(systemMsgs)

	remainingBudget := maxTokens - systemTokens
	if remainingBudget <= 0 {
		logger.SugaredLogger.Warnf("MessageRewriter: 系统消息已超出预算, 仅保留系统消息")
		return result
	}

	compressed := compressNonSystemMessages(nonSystemMsgs, remainingBudget)
	result = append(result, compressed...)

	finalTokens := estimateMessagesTokens(result)
	logger.SugaredLogger.Infof("MessageRewriter: 上下文压缩后 %d 条消息, 估算 %d tokens (节省 %d tokens)",
		len(result), finalTokens, currentTokens-finalTokens)

	return result
}

func compressNonSystemMessages(messages []*schema.Message, maxTokens int) []*schema.Message {
	if len(messages) == 0 {
		return messages
	}

	trimmed := trimLargeToolResults(messages, 4000)

	currentTokens := estimateMessagesTokens(trimmed)
	if currentTokens <= maxTokens {
		return trimmed
	}

	var userMsgs []*schema.Message
	var otherMsgs []*schema.Message
	for _, msg := range trimmed {
		if msg.Role == schema.User {
			userMsgs = append(userMsgs, msg)
		} else {
			otherMsgs = append(otherMsgs, msg)
		}
	}

	if len(userMsgs) > 0 {
		lastUserMsg := userMsgs[len(userMsgs)-1]
		lastUserTokens := estimateTokens(lastUserMsg.Content) + 4

		otherBudget := maxTokens - lastUserTokens
		if otherBudget < 0 {
			otherBudget = 0
		}

		compressedOthers := dropOldestMessages(otherMsgs, otherBudget)
		result := make([]*schema.Message, 0, len(compressedOthers)+1)
		result = append(result, compressedOthers...)
		result = append(result, lastUserMsg)
		return result
	}

	return dropOldestMessages(trimmed, maxTokens)
}

func trimLargeToolResults(messages []*schema.Message, maxToolResultTokens int) []*schema.Message {
	result := make([]*schema.Message, len(messages))
	for i, msg := range messages {
		if msg.Role == schema.Tool && estimateTokens(msg.Content) > maxToolResultTokens {
			compressed := trimToolResult(msg.Content, maxToolResultTokens)
			result[i] = &schema.Message{
				Role:       msg.Role,
				Content:    compressed,
				ToolCallID: msg.ToolCallID,
				ToolName:   msg.ToolName,
				Name:       msg.Name,
			}
		} else {
			result[i] = msg
		}
	}
	return result
}

func dropOldestMessages(messages []*schema.Message, maxTokens int) []*schema.Message {
	if len(messages) == 0 {
		return messages
	}

	totalTokens := estimateMessagesTokens(messages)
	if totalTokens <= maxTokens {
		return messages
	}

	toolCallIDs := make(map[string]bool)
	for _, msg := range messages {
		if msg.Role == schema.Assistant {
			for _, tc := range msg.ToolCalls {
				toolCallIDs[tc.ID] = true
			}
		}
	}

	kept := make([]*schema.Message, 0, len(messages))
	tokenSum := 0
	started := false

	for i, msg := range messages {
		if !started {
			if msg.Role == schema.Assistant && len(msg.ToolCalls) > 0 {
				assistantTokens := estimateTokens(msg.Content)
				for _, tc := range msg.ToolCalls {
					assistantTokens += estimateTokens(tc.Function.Name) + estimateTokens(tc.Function.Arguments)
				}
				assistantTokens += 4

				var relatedToolMsgs []*schema.Message
				relatedTokens := 0
				for j := i + 1; j < len(messages); j++ {
					if messages[j].Role == schema.Tool {
						for _, tc := range msg.ToolCalls {
							if messages[j].ToolCallID == tc.ID {
								relatedToolMsgs = append(relatedToolMsgs, messages[j])
								relatedTokens += estimateTokens(messages[j].Content) + 4
								break
							}
						}
					}
				}

				groupTokens := assistantTokens + relatedTokens
				if tokenSum+groupTokens <= maxTokens {
					started = true
					kept = append(kept, msg)
					tokenSum += groupTokens
					kept = append(kept, relatedToolMsgs...)
				}
			} else if msg.Role == schema.Tool {
				if toolCallIDs[msg.ToolCallID] {
					continue
				}
			} else {
				msgTokens := estimateTokens(msg.Content) + 4
				if tokenSum+msgTokens <= maxTokens {
					started = true
					kept = append(kept, msg)
					tokenSum += msgTokens
				}
			}
			continue
		}

		msgTokens := estimateTokens(msg.Content) + 4
		for _, tc := range msg.ToolCalls {
			msgTokens += estimateTokens(tc.Function.Name) + estimateTokens(tc.Function.Arguments)
		}

		if msg.Role == schema.Assistant && len(msg.ToolCalls) > 0 {
			var relatedToolMsgs []*schema.Message
			for j := i + 1; j < len(messages); j++ {
				if messages[j].Role == schema.Tool {
					for _, tc := range msg.ToolCalls {
						if messages[j].ToolCallID == tc.ID {
							relatedToolMsgs = append(relatedToolMsgs, messages[j])
							msgTokens += estimateTokens(messages[j].Content) + 4
							break
						}
					}
				}
			}

			if tokenSum+msgTokens > maxTokens {
				break
			}
			kept = append(kept, msg)
			tokenSum += msgTokens
			kept = append(kept, relatedToolMsgs...)
		} else if msg.Role == schema.Tool {
			if !toolCallIDs[msg.ToolCallID] {
				if tokenSum+msgTokens <= maxTokens {
					kept = append(kept, msg)
					tokenSum += msgTokens
				}
			} else {
				kept = append(kept, msg)
				tokenSum += msgTokens
			}
		} else {
			if tokenSum+msgTokens > maxTokens {
				break
			}
			kept = append(kept, msg)
			tokenSum += msgTokens
		}
	}

	if len(kept) == 0 && len(messages) > 0 {
		last := messages[len(messages)-1]
		kept = append(kept, last)
	}

	return kept
}

func isTokenLimitError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
	return strings.Contains(errMsg, "max_prompt_tokens") ||
		strings.Contains(errMsg, "exceeded") ||
		strings.Contains(errMsg, "token limit") ||
		strings.Contains(errMsg, "context_length") ||
		strings.Contains(errMsg, "maximum context") ||
		strings.Contains(errMsg, "too many tokens") ||
		strings.Contains(errMsg, "context window") ||
		strings.Contains(errMsg, "400") && strings.Contains(errMsg, "token")
}
