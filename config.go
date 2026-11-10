package main

import (
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/viper"
)

// ProjectConfig 单个项目的部署配置。
type ProjectConfig struct {
	Name                 string   `json:"name" mapstructure:"name"`
	Path                 string   `json:"path" mapstructure:"path"`
	Branch               string   `json:"branch" mapstructure:"branch"`
	Script               string   `json:"script" mapstructure:"script"`
	PackageManager       string   `json:"packageManager" mapstructure:"package-manager"`
	Dist                 string   `json:"dist" mapstructure:"dist"`
	Output               string   `json:"output" mapstructure:"output"`
	Token                string   `json:"token" mapstructure:"token"`
	AdditionalScript     string   `json:"additionalScript" mapstructure:"additional-script"`
	AdditionalScriptArgs []string `json:"additionalScriptArgs" mapstructure:"additional-script-args"`
}

// AppConfig 应用全局配置。
type AppConfig struct {
	Port     string          `json:"port" mapstructure:"port"`
	Host     string          `json:"host" mapstructure:"host"`
	Password string          `json:"-" mapstructure:"password"`
	Token    string          `json:"-" mapstructure:"token"`
	LogDir   string          `json:"logDir" mapstructure:"log-dir"`
	Projects []ProjectConfig `json:"projects" mapstructure:"projects"`
}

var projectNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// Validate 校验项目配置。
func (c *ProjectConfig) Validate() error {
	c.Name = strings.TrimSpace(c.Name)
	c.Path = strings.TrimSpace(c.Path)
	if c.Name == "" {
		return fmt.Errorf("项目名不能为空")
	}
	if !projectNamePattern.MatchString(c.Name) {
		return fmt.Errorf("项目名只能包含字母、数字、下划线和连字符: %q", c.Name)
	}
	if c.Path == "" {
		return fmt.Errorf("仓库路径不能为空")
	}
	fi, err := os.Stat(c.Path)
	if err != nil {
		return fmt.Errorf("仓库路径不可访问: %v", err)
	}
	if !fi.IsDir() {
		return fmt.Errorf("仓库路径不是目录: %s", c.Path)
	}
	return nil
}

// LoadConfig 从指定路径读取配置文件。
func LoadConfig(path string) (*AppConfig, error) {
	v := viper.New()
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}
	cfg := &AppConfig{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}
	if cfg.Port == "" {
		cfg.Port = "8079"
	}
	cfg.Port = strings.TrimPrefix(cfg.Port, ":")
	cfg.Host = strings.TrimSpace(cfg.Host)
	if err := cfg.validateHost(); err != nil {
		return nil, err
	}
	if cfg.LogDir == "" {
		cfg.LogDir = "logs"
	}

	seen := map[string]bool{}
	for i := range cfg.Projects {
		if err := cfg.Projects[i].Validate(); err != nil {
			return nil, fmt.Errorf("项目 %q 配置无效: %w", cfg.Projects[i].Name, err)
		}
		if seen[cfg.Projects[i].Name] {
			return nil, fmt.Errorf("项目名重复: %q", cfg.Projects[i].Name)
		}
		seen[cfg.Projects[i].Name] = true
	}
	return cfg, nil
}

// validateHost 校验监听地址配置:
// 合法取值为空(全部地址)、"*"/"all"、具体 IP(v4/v6)以及本机回环地址(localhost/127.x/::1 等)。
func (c *AppConfig) validateHost() error {
	h := strings.ToLower(c.Host)
	if h == "" || h == "*" || h == "all" {
		return nil
	}
	if h == "localhost" {
		return nil
	}
	if ip := net.ParseIP(c.Host); ip != nil {
		return nil
	}
	return fmt.Errorf("host 配置无效: %q(应为空/本地回环地址/具体 IP,示例:0.0.0.0、127.0.0.1、192.168.1.10、::1)", c.Host)
}

// ListenAddr 拼出监听地址:
//   - host 为空 / "*" / "all" / "0.0.0.0" → ":port",监听全部地址(IPv4+IPv6)
//   - host 为 "localhost" → 127.0.0.1:port
//   - 其他 → 指定 IP:port(IPv6 自动加方括号)
func (c *AppConfig) ListenAddr() string {
	h := strings.TrimSpace(c.Host)
	switch strings.ToLower(h) {
	case "", "*", "all", "0.0.0.0":
		return ":" + c.Port
	case "localhost":
		h = "127.0.0.1"
	}
	return net.JoinHostPort(h, c.Port)
}

// ListenDisplayAddr 启动提示用的可访问地址。
func (c *AppConfig) ListenDisplayAddr() string {
	addr := c.ListenAddr()
	if strings.HasPrefix(addr, ":") {
		return "localhost" + addr
	}
	return addr
}

// SaveConfig 将配置整体写回 TOML 文件(先写临时文件再原子替换)。
func SaveConfig(cfg *AppConfig, path string) error {
	content := RenderTOML(cfg)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
		return fmt.Errorf("写入配置失败: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("替换配置文件失败: %w", err)
	}
	return nil
}

// RenderTOML 将配置渲染为带注释的 TOML 文本。
func RenderTOML(cfg *AppConfig) string {
	var b strings.Builder
	b.WriteString("# webhook-service 配置文件(由管理界面维护,手动编辑后需重启服务)\n\n")
	b.WriteString("# Web 界面与 API 监听端口\n")
	fmt.Fprintf(&b, "port = %s\n", tomlQuote(cfg.Port))
	b.WriteString("# 监听地址:留空或 0.0.0.0 = 全部地址;localhost / 127.0.0.1 = 仅本机;也可填具体 IP 如 192.168.1.10\n")
	fmt.Fprintf(&b, "host = %s\n", tomlQuote(cfg.Host))
	b.WriteString("# Web 界面登录密码,留空则免登录\n")
	fmt.Fprintf(&b, "password = %s\n", tomlQuote(cfg.Password))
	b.WriteString("# 全局 webhook token,项目未单独配置 token 时使用\n")
	fmt.Fprintf(&b, "token = %s\n", tomlQuote(cfg.Token))
	b.WriteString("# 日志目录\n")
	fmt.Fprintf(&b, "log-dir = %s\n", tomlQuote(cfg.LogDir))

	for _, p := range cfg.Projects {
		b.WriteString("\n# 项目\n[[projects]]\n")
		fmt.Fprintf(&b, "name = %s\n", tomlQuote(p.Name))
		fmt.Fprintf(&b, "path = %s\n", tomlQuote(p.Path))
		fmt.Fprintf(&b, "branch = %s\n", tomlQuote(p.Branch))
		fmt.Fprintf(&b, "script = %s\n", tomlQuote(p.Script))
		fmt.Fprintf(&b, "package-manager = %s\n", tomlQuote(p.PackageManager))
		fmt.Fprintf(&b, "dist = %s\n", tomlQuote(p.Dist))
		fmt.Fprintf(&b, "output = %s\n", tomlQuote(p.Output))
		fmt.Fprintf(&b, "token = %s\n", tomlQuote(p.Token))
		fmt.Fprintf(&b, "additional-script = %s\n", tomlQuote(p.AdditionalScript))
		b.WriteString("additional-script-args = [")
		for i, a := range p.AdditionalScriptArgs {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(tomlQuote(a))
		}
		b.WriteString("]\n")
	}
	return b.String()
}

// tomlQuote 转义为 TOML 基本字符串。
func tomlQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(&b, `\u%04X`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}
