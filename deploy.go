package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// execute 执行一次部署流水线:
// 清理工作区 → 拉取代码 → 附加脚本 → 构建 → 搬运产物。
// 任一步骤失败立即中断,绝不继续执行清空/移动目录的部署步骤。
func (p *Project) execute(depID string) error {
	cfg := p.Cfg

	p.Sink.Log("info", "clean", depID, "清理工作区(git clean / reset)...")
	if err := GitClean(cfg.Path, p.Sink, depID); err != nil {
		return fmt.Errorf("git clean 失败: %w", err)
	}

	p.Sink.Log("info", "pull", depID, "拉取最新代码(git pull)...")
	if err := GitPull(cfg.Path, p.Sink, depID); err != nil {
		return fmt.Errorf("git pull 失败: %w", err)
	}

	if cfg.AdditionalScript != "" {
		p.Sink.Log("info", "script", depID, "执行附加脚本 "+cfg.AdditionalScript+"...")
		if err := RunAdditionScript(cfg.Path, cfg.AdditionalScript, cfg.AdditionalScriptArgs, p.Sink, depID); err != nil {
			return fmt.Errorf("附加脚本执行失败: %w", err)
		}
	}

	p.Sink.Log("info", "build", depID, "开始构建...")
	if err := RunBuild(cfg.Path, cfg.Script, cfg.PackageManager, p.Sink, depID); err != nil {
		return fmt.Errorf("构建失败: %w", err)
	}

	p.Sink.Log("info", "deploy", depID, "发布产物...")
	if err := ClearTargetFolderThenMove(cfg.Path, cfg.Output, cfg.Dist, p.Sink, depID); err != nil {
		return fmt.Errorf("发布失败: %w", err)
	}

	return nil
}

// newDeploymentID 生成部署会话 ID:日期时间 + 随机后缀。
func newDeploymentID() string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return time.Now().Format("20060102-150405") + "-" + hex.EncodeToString(b)
}

// formatDuration 格式化耗时。
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return d.Round(time.Millisecond).String()
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm%ds", m, s)
}
