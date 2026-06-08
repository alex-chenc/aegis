package assets

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

// LLMServiceCollector LLM 服务探测器
// 参考 Julius (https://github.com/Praetorian-Inc/julius) 的 Probe 架构
// 对监听端口的进程发送 HTTP 探测请求，识别 LLM 推理服务
type LLMServiceCollector struct {
	logger *zap.Logger
	client *http.Client
	probes []LLMProbe
}

// LLMProbe LLM 服务探测规则
type LLMProbe struct {
	Name        string        // 服务标识 (如 "ollama")
	DisplayName string        // 显示名称 (如 "Ollama")
	PortHints   []int         // 常见端口
	Requests    []ProbeRequest // 探测请求列表
	MatchMode   string        // "all" 或 "any"
}

// ProbeRequest 单个探测请求
type ProbeRequest struct {
	Path         string
	Method       string // 默认 GET
	BodyContains []string
	StatusCodes  []int
}

// llmProbeResult 内部探测结果
type llmProbeResult struct {
	probe    LLMProbe
	endpoint string
	port     int
	version  string
	extra    map[string]string
}

// NewLLMServiceCollector 创建 LLM 服务探测器
func NewLLMServiceCollector(logger *zap.Logger) *LLMServiceCollector {
	return &LLMServiceCollector{
		logger: logger,
		client: &http.Client{Timeout: 3 * time.Second},
		probes: defaultLLMProbes(),
	}
}

// Collect 对已知端口执行 LLM 服务探测
func (c *LLMServiceCollector) Collect(ctx context.Context, listenPorts map[int][]int) []AIAsset {
	// listenPorts: port -> []pids
	if len(listenPorts) == 0 {
		return nil
	}

	var results []AIAsset
	seen := make(map[string]bool) // 去重: probeName:endpoint

	for _, probe := range c.probes {
		for _, port := range probe.PortHints {
			pids, ok := listenPorts[port]
			if !ok {
				continue
			}

			endpoint := fmt.Sprintf("http://127.0.0.1:%d", port)
			key := probe.Name + ":" + endpoint
			if seen[key] {
				continue
			}

			matched, version, extra := c.matchProbe(ctx, probe, endpoint)
			if !matched {
				continue
			}
			seen[key] = true

			asset := AIAsset{
				Category:    "llm_service",
				Name:        probe.Name,
				DisplayName: probe.DisplayName,
				Version:     version,
				Source:      "probe",
				Endpoint:    endpoint,
				ListenPorts: []int{port},
				PIDs:        pids,
				Extra:       extra,
			}
			results = append(results, asset)
			c.logger.Info("LLM service detected",
				zap.String("service", probe.Name),
				zap.Int("port", port),
				zap.String("version", version))
		}
	}

	return results
}

// matchProbe 执行探测并匹配响应
func (c *LLMServiceCollector) matchProbe(ctx context.Context, probe LLMProbe, endpoint string) (bool, string, map[string]string) {
	extra := make(map[string]string)
	version := ""
	matchedCount := 0

	for _, req := range probe.Requests {
		matched, ver := c.doRequest(ctx, endpoint, req)
		if matched {
			matchedCount++
			if ver != "" && version == "" {
				version = ver
			}
		} else if probe.MatchMode == "all" {
			return false, "", nil
		}
	}

	if probe.MatchMode == "all" {
		return matchedCount == len(probe.Requests), version, extra
	}
	return matchedCount > 0, version, extra
}

// doRequest 执行单个探测请求
func (c *LLMServiceCollector) doRequest(ctx context.Context, endpoint string, req ProbeRequest) (bool, string) {
	method := req.Method
	if method == "" {
		method = "GET"
	}

	url := endpoint + req.Path
	httpReq, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return false, ""
	}

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return false, ""
	}
	defer resp.Body.Close()

	// 检查状态码
	if len(req.StatusCodes) > 0 {
		codeMatch := false
		for _, code := range req.StatusCodes {
			if resp.StatusCode == code {
				codeMatch = true
				break
			}
		}
		if !codeMatch {
			return false, ""
		}
	}

	// 读取 body（限制 1MB）
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return false, ""
	}
	bodyStr := string(body)

	// 检查 body 包含条件
	for _, contain := range req.BodyContains {
		if !strings.Contains(bodyStr, contain) {
			return false, ""
		}
	}

	// 尝试提取版本
	version := extractVersion(bodyStr)

	return true, version
}

// extractVersion 从响应 body 中提取版本号
func extractVersion(body string) string {
	// 常见版本字段模式
	patterns := []struct {
		key string
	}{
		{`"version":"`},
		{`"version": "`},
		{`"litellm_version":"`},
	}

	for _, p := range patterns {
		idx := strings.Index(body, p.key)
		if idx < 0 {
			continue
		}
		start := idx + len(p.key)
		end := strings.IndexAny(body[start:], `",}`)
		if end > 0 {
			return body[start : start+end]
		}
	}
	return ""
}

// defaultLLMProbes 返回默认的 LLM 服务探测规则
// 参考 Julius 的 63 个 Probe，选取最常见的服务
func defaultLLMProbes() []LLMProbe {
	return []LLMProbe{
		{
			Name:        "ollama",
			DisplayName: "Ollama",
			PortHints:   []int{11434},
			MatchMode:   "all",
			Requests: []ProbeRequest{
				{Path: "/", StatusCodes: []int{200}, BodyContains: []string{"Ollama is running"}},
				{Path: "/api/tags", StatusCodes: []int{200}, BodyContains: []string{"models"}},
			},
		},
		{
			Name:        "vllm",
			DisplayName: "vLLM",
			PortHints:   []int{8000},
			MatchMode:   "all",
			Requests: []ProbeRequest{
				{Path: "/v1/models", StatusCodes: []int{200}, BodyContains: []string{"data"}},
			},
		},
		{
			Name:        "litellm",
			DisplayName: "LiteLLM",
			PortHints:   []int{4000},
			MatchMode:   "any",
			Requests: []ProbeRequest{
				{Path: "/health", StatusCodes: []int{200}, BodyContains: []string{"litellm_metadata"}},
			},
		},
		{
			Name:        "sglang",
			DisplayName: "SGLang",
			PortHints:   []int{30000},
			MatchMode:   "all",
			Requests: []ProbeRequest{
				{Path: "/v1/models", StatusCodes: []int{200}, BodyContains: []string{"data"}},
			},
		},
		{
			Name:        "localai",
			DisplayName: "LocalAI",
			PortHints:   []int{8080},
			MatchMode:   "all",
			Requests: []ProbeRequest{
				{Path: "/v1/models", StatusCodes: []int{200}, BodyContains: []string{"data"}},
			},
		},
		{
			Name:        "llama-cpp",
			DisplayName: "llama.cpp",
			PortHints:   []int{8080},
			MatchMode:   "any",
			Requests: []ProbeRequest{
				{Path: "/health", StatusCodes: []int{200}, BodyContains: []string{"ok"}},
			},
		},
		{
			Name:        "tgi",
			DisplayName: "HuggingFace TGI",
			PortHints:   []int{8080},
			MatchMode:   "any",
			Requests: []ProbeRequest{
				{Path: "/health", StatusCodes: []int{200}, BodyContains: []string{"ok"}},
			},
		},
		{
			Name:        "nvidia-nim",
			DisplayName: "NVIDIA NIM",
			PortHints:   []int{8000},
			MatchMode:   "all",
			Requests: []ProbeRequest{
				{Path: "/v1/models", StatusCodes: []int{200}, BodyContains: []string{"data"}},
			},
		},
		{
			Name:        "dify",
			DisplayName: "Dify",
			PortHints:   []int{5001, 3000},
			MatchMode:   "any",
			Requests: []ProbeRequest{
				{Path: "/apps", StatusCodes: []int{200}, BodyContains: []string{"Dify"}},
				{Path: "/", StatusCodes: []int{200}, BodyContains: []string{"Dify"}},
			},
		},
		{
			Name:        "open-webui",
			DisplayName: "Open WebUI",
			PortHints:   []int{3000, 8080},
			MatchMode:   "any",
			Requests: []ProbeRequest{
				{Path: "/api/v1/auths/", StatusCodes: []int{200, 401, 403}},
			},
		},
		{
			Name:        "flowise",
			DisplayName: "Flowise",
			PortHints:   []int{3000},
			MatchMode:   "any",
			Requests: []ProbeRequest{
				{Path: "/api/v1/chatflows", StatusCodes: []int{200, 401}},
			},
		},
		{
			Name:        "langflow",
			DisplayName: "Langflow",
			PortHints:   []int{7860},
			MatchMode:   "any",
			Requests: []ProbeRequest{
				{Path: "/api/v1/all", StatusCodes: []int{200}},
			},
		},
		{
			Name:        "lmstudio",
			DisplayName: "LM Studio",
			PortHints:   []int{1234},
			MatchMode:   "all",
			Requests: []ProbeRequest{
				{Path: "/v1/models", StatusCodes: []int{200}, BodyContains: []string{"data"}},
			},
		},
		{
			Name:        "koboldcpp",
			DisplayName: "KoboldCpp",
			PortHints:   []int{5001},
			MatchMode:   "all",
			Requests: []ProbeRequest{
				{Path: "/api/v1/model", StatusCodes: []int{200}, BodyContains: []string{"result"}},
			},
		},
		{
			Name:        "openai-compatible",
			DisplayName: "OpenAI Compatible API",
			PortHints:   []int{8000, 8080, 3000, 5000, 4000, 1234, 11434},
			MatchMode:   "any",
			Requests: []ProbeRequest{
				{Path: "/v1/models", StatusCodes: []int{200, 401, 403}},
			},
		},
	}
}
