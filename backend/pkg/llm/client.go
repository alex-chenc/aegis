package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type Client struct {
	BaseURL    string
	APIKey     string
	Model      string
	HTTPClient *http.Client
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatCompletionRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
}

type ChatCompletionResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type ParsedRule struct {
	Title        string `json:"title"`
	CheckContent string `json:"check_content"`
	FixContent   string `json:"fix_content"`
}

func NewClient(apiKey, baseURL string) *Client {
	if baseURL == "" {
		baseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	}
	return &Client{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Model:   "qwen-plus",
		HTTPClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func NewClientFromEnv() *Client {
	apiKey := os.Getenv("LLM_API_KEY")
	baseURL := os.Getenv("LLM_BASE_URL")
	return NewClient(apiKey, baseURL)
}

func (c *Client) Chat(messages []Message) (string, error) {
	req := ChatCompletionRequest{
		Model:    c.Model,
		Messages: messages,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("LLM API error: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	var chatResp ChatCompletionResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return chatResp.Choices[0].Message.Content, nil
}

func (c *Client) TestConnection() error {
	_, err := c.Chat([]Message{
		{Role: "user", Content: "Hello"},
	})
	return err
}

func (c *Client) ParseBaselineDocument(content string) ([]ParsedRule, error) {
	systemPrompt := `你是一个基线安全检查文档解析专家。请分析用户提供的基线检查文档，提取出所有检查规则。

对于每条规则，请提取以下信息：
1. title: 规则的简短标题
2. check_content: 检查内容描述（如何检查）
3. fix_content: 修复内容描述（如何修复）

请以JSON数组格式返回，格式如下：
[
  {
    "title": "检查SSH配置",
    "check_content": "检查/etc/ssh/sshd_config文件中PermitRootLogin配置项",
    "fix_content": "修改PermitRootLogin为no"
  }
]

只返回JSON数组，不要有其他内容。`

	userPrompt := fmt.Sprintf("请解析以下基线检查文档：\n\n%s", content)

	response, err := c.Chat([]Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	})
	if err != nil {
		return nil, err
	}

	var rules []ParsedRule
	if err := json.Unmarshal([]byte(response), &rules); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response as JSON: %w, response: %s", err, response)
	}

	return rules, nil
}

func (c *Client) GenerateCheckScript(ruleTitle, checkContent string) (string, error) {
	systemPrompt := `你是一个Shell脚本编写专家。请根据用户提供的基线检查规则，编写一个Shell脚本来执行检查。

要求：
1. 脚本应该是可执行的Bash脚本
2. 检查通过时退出码为0，输出合规信息
3. 检查不通过时退出码为1，输出不合规信息及具体原因
4. 只返回脚本内容，不要有其他解释`

	userPrompt := fmt.Sprintf("规则标题：%s\n检查内容：%s\n\n请编写检查脚本：", ruleTitle, checkContent)

	return c.Chat([]Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	})
}

func (c *Client) GenerateFixScript(ruleTitle, fixContent string) (string, error) {
	systemPrompt := `你是一个Shell脚本编写专家。请根据用户提供的基线修复规则，编写一个Shell脚本来执行修复。

要求：
1. 脚本应该是可执行的Bash脚本
2. 脚本执行修复操作，修复成功时退出码为0
3. 修复失败时退出码为1，输出错误信息
4. 只返回脚本内容，不要有其他解释`

	userPrompt := fmt.Sprintf("规则标题：%s\n修复内容：%s\n\n请编写修复脚本：", ruleTitle, fixContent)

	return c.Chat([]Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	})
}
