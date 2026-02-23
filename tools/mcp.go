package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	log "xbot/logger"

	"xbot/llm"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

// MCPServerConfig 单个 MCP Server 的配置
type MCPServerConfig struct {
	Command string            `json:"command"`           // 可执行文件路径
	Args    []string          `json:"args,omitempty"`    // 命令行参数
	Env     map[string]string `json:"env,omitempty"`     // 环境变量
	Enabled *bool             `json:"enabled,omitempty"` // 是否启用（默认 true）
}

// MCPConfig 整体 MCP 配置（从 mcp.json 读取）
type MCPConfig struct {
	MCPServers map[string]MCPServerConfig `json:"mcpServers"`
}

// mcpConnection 一个已连接的 MCP Server
type mcpConnection struct {
	name      string
	client    *mcpclient.Client
	transport *transport.Stdio
	tools     []mcp.Tool
}

// MCPManager 管理所有 MCP Server 连接
type MCPManager struct {
	mu          sync.RWMutex
	connections map[string]*mcpConnection
	configPath  string
}

// NewMCPManager 创建 MCPManager
func NewMCPManager(configPath string) *MCPManager {
	return &MCPManager{
		connections: make(map[string]*mcpConnection),
		configPath:  configPath,
	}
}

// LoadAndConnect 加载配置并连接所有 MCP Server
func (m *MCPManager) LoadAndConnect(ctx context.Context) error {
	config, err := m.loadConfig()
	if err != nil {
		if os.IsNotExist(err) {
			log.Debug("No mcp.json found, skipping MCP initialization")
			return nil
		}
		return fmt.Errorf("load mcp config: %w", err)
	}

	for name, serverCfg := range config.MCPServers {
		if serverCfg.Enabled != nil && !*serverCfg.Enabled {
			log.WithField("server", name).Info("MCP server disabled, skipping")
			continue
		}

		if err := m.connectServer(ctx, name, serverCfg); err != nil {
			log.WithError(err).WithField("server", name).Error("Failed to connect MCP server")
			continue
		}
	}

	return nil
}

// connectServer 连接单个 MCP Server
func (m *MCPManager) connectServer(ctx context.Context, name string, cfg MCPServerConfig) error {
	// 构建环境变量列表
	var envList []string
	for k, v := range cfg.Env {
		envList = append(envList, fmt.Sprintf("%s=%s", k, v))
	}

	// 创建 stdio transport
	stdioTransport := transport.NewStdio(cfg.Command, envList, cfg.Args...)

	// 启动 transport
	connectCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := stdioTransport.Start(connectCtx); err != nil {
		return fmt.Errorf("start transport: %w", err)
	}

	// 创建 client
	client := mcpclient.NewClient(stdioTransport)

	// 初始化 MCP 协议
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "xbot",
		Version: "1.0.0",
	}

	_, err := client.Initialize(connectCtx, initReq)
	if err != nil {
		stdioTransport.Close()
		return fmt.Errorf("initialize: %w", err)
	}

	// 获取可用工具列表
	toolsResult, err := client.ListTools(connectCtx, mcp.ListToolsRequest{})
	if err != nil {
		stdioTransport.Close()
		return fmt.Errorf("list tools: %w", err)
	}

	conn := &mcpConnection{
		name:      name,
		client:    client,
		transport: stdioTransport,
		tools:     toolsResult.Tools,
	}

	m.mu.Lock()
	m.connections[name] = conn
	m.mu.Unlock()

	toolNames := make([]string, len(conn.tools))
	for i, t := range conn.tools {
		toolNames[i] = t.Name
	}

	log.WithFields(log.Fields{
		"server": name,
		"tools":  toolNames,
	}).Infof("MCP server connected (%d tools)", len(conn.tools))

	return nil
}

// RegisterTools 将所有 MCP 远程工具注册到 Registry
func (m *MCPManager) RegisterTools(registry *Registry) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, conn := range m.connections {
		for _, tool := range conn.tools {
			remoteTool := newMCPRemoteTool(conn.name, tool, conn.client)
			registry.Register(remoteTool)
		}
	}
}

// Close 关闭所有 MCP 连接
func (m *MCPManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, conn := range m.connections {
		if err := conn.transport.Close(); err != nil {
			log.WithError(err).WithField("server", name).Warn("Error closing MCP connection")
		}
	}
	m.connections = make(map[string]*mcpConnection)
}

// ServerCount 返回已连接的 MCP Server 数量
func (m *MCPManager) ServerCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.connections)
}

// loadConfig 从 JSON 文件加载 MCP 配置
func (m *MCPManager) loadConfig() (*MCPConfig, error) {
	data, err := os.ReadFile(m.configPath)
	if err != nil {
		return nil, err
	}

	var config MCPConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse mcp.json: %w", err)
	}
	return &config, nil
}

// ---- MCPRemoteTool: 将 MCP 远程工具适配为 xbot Tool 接口 ----

// MCPRemoteTool 封装一个远程 MCP 工具为 xbot Tool
type MCPRemoteTool struct {
	serverName  string
	tool        mcp.Tool
	client      *mcpclient.Client
	params      []llm.ToolParam
	description string
}

// newMCPRemoteTool 创建 MCPRemoteTool
func newMCPRemoteTool(serverName string, tool mcp.Tool, client *mcpclient.Client) *MCPRemoteTool {
	params := convertMCPParams(tool)
	desc := tool.Description
	if desc == "" {
		desc = fmt.Sprintf("MCP tool from %s", serverName)
	}

	return &MCPRemoteTool{
		serverName:  serverName,
		tool:        tool,
		client:      client,
		params:      params,
		description: desc,
	}
}

func (t *MCPRemoteTool) Name() string {
	// 添加 server 前缀避免工具名冲突
	return fmt.Sprintf("mcp_%s_%s", t.serverName, t.tool.Name)
}

func (t *MCPRemoteTool) Description() string {
	return fmt.Sprintf("[MCP:%s] %s", t.serverName, t.description)
}

func (t *MCPRemoteTool) Parameters() []llm.ToolParam {
	return t.params
}

func (t *MCPRemoteTool) Execute(ctx *ToolContext, input string) (*ToolResult, error) {
	// 解析 JSON 参数为 map
	var args map[string]any
	if input != "" && input != "{}" {
		if err := json.Unmarshal([]byte(input), &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}

	// 构建 MCP CallToolRequest
	req := mcp.CallToolRequest{}
	req.Params.Name = t.tool.Name
	req.Params.Arguments = args

	// 调用远程工具
	result, err := t.client.CallTool(ctx.Ctx, req)
	if err != nil {
		return nil, fmt.Errorf("MCP call %s/%s: %w", t.serverName, t.tool.Name, err)
	}

	// 将 MCP 结果转为字符串
	content := formatMCPResult(result)

	if result.IsError {
		return nil, fmt.Errorf("MCP tool error: %s", content)
	}

	return NewResult(content), nil
}

// convertMCPParams 将 MCP Tool 的 JSON Schema 参数转为 xbot ToolParam 列表
func convertMCPParams(tool mcp.Tool) []llm.ToolParam {
	schema := tool.InputSchema
	props := schema.Properties
	if props == nil {
		return nil
	}

	// 构建 required 集合
	requiredSet := make(map[string]bool)
	for _, r := range schema.Required {
		requiredSet[r] = true
	}

	var params []llm.ToolParam
	for name, propRaw := range props {
		// propRaw 是 interface{}，通常是 map[string]interface{}
		propMap, ok := propRaw.(map[string]interface{})
		if !ok {
			params = append(params, llm.ToolParam{
				Name:     name,
				Type:     "string",
				Required: requiredSet[name],
			})
			continue
		}

		paramType := "string"
		if t, ok := propMap["type"].(string); ok {
			paramType = t
		}

		desc := ""
		if d, ok := propMap["description"].(string); ok {
			desc = d
		}

		// 如果有 enum，附加到描述
		if enumVals, ok := propMap["enum"].([]interface{}); ok && len(enumVals) > 0 {
			enumStrs := make([]string, len(enumVals))
			for i, v := range enumVals {
				enumStrs[i] = fmt.Sprintf("%v", v)
			}
			desc += fmt.Sprintf(" (options: %s)", strings.Join(enumStrs, ", "))
		}

		params = append(params, llm.ToolParam{
			Name:        name,
			Type:        paramType,
			Description: desc,
			Required:    requiredSet[name],
		})
	}
	return params
}

// formatMCPResult 将 MCP CallToolResult 的 Content 转为文本
func formatMCPResult(result *mcp.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return "(no output)"
	}

	var parts []string
	for _, c := range result.Content {
		switch v := c.(type) {
		case mcp.TextContent:
			parts = append(parts, v.Text)
		case *mcp.TextContent:
			parts = append(parts, v.Text)
		case mcp.ImageContent:
			parts = append(parts, fmt.Sprintf("[image: %s]", v.MIMEType))
		case *mcp.ImageContent:
			parts = append(parts, fmt.Sprintf("[image: %s]", v.MIMEType))
		case mcp.AudioContent:
			parts = append(parts, fmt.Sprintf("[audio: %s]", v.MIMEType))
		case *mcp.AudioContent:
			parts = append(parts, fmt.Sprintf("[audio: %s]", v.MIMEType))
		case mcp.EmbeddedResource:
			data, _ := json.Marshal(v)
			parts = append(parts, string(data))
		case *mcp.EmbeddedResource:
			data, _ := json.Marshal(v)
			parts = append(parts, string(data))
		default:
			data, _ := json.Marshal(c)
			parts = append(parts, string(data))
		}
	}
	return strings.Join(parts, "\n")
}
