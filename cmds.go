package main

import (
	"bufio"
	"errors"
	"io"
	"os/exec"
	"strings"

	"github.com/kataras/golog"
)

// RunCmd cmd 进程通用方法
func RunCmd(c *exec.Cmd, cType string, logger *golog.Logger) error {
	released := false

	stdout, err := c.StdoutPipe()
	if err != nil {
		return err
	}
	stdErrOut, err := c.StderrPipe()
	if err != nil {
		return err
	}

	if err := c.Start(); err != nil {
		return err
	}

	released = doScan(c, stdout, logger, false)
	released = doScan(c, stdErrOut, logger, true)

	if !released {
		if err := c.Wait(); err != nil {
			logger.Error("command wait error:", err)
			return err
		}
	} else {
		return errors.New("command error")
	}

	return nil
}

func doScan(c *exec.Cmd, stdout io.ReadCloser, logger *golog.Logger, isError bool) bool {
	released := false
	scanner := bufio.NewScanner(stdout)
	scanner.Split(bufio.ScanLines)
Loop:
	for scanner.Scan() {
		m := scanner.Text()
		logger.Info(m)
		if strings.Contains(m, "exit code") {
			if err := c.Process.Release(); err != nil {
				panic(err)
			}
			released = true
			logger.Info("process released")
			break Loop
		}
	}
	return released
}

// GitClean run cmd git clean.
func GitClean(logger *golog.Logger) error {
	gitPullCmd := exec.Command("git", "clean", "-f")
	if err := RunCmd(gitPullCmd, "clean", logger); err != nil {
		return err
	}

	return nil
}

// GitPull run cmd git pull.
func GitPull(logger *golog.Logger) error {
	gitPullCmd := exec.Command("git", "pull")
	if err := RunCmd(gitPullCmd, "pull", logger); err != nil {
		return err
	}

	return nil
}

// RunBuild run cmd npm run build.
func RunBuild(logger *golog.Logger, script string, manager string) error {
	var str string

	str = script
	if script == "" {
		str = "build"
	}

	if manager == "" {
		manager = "npm"
	}
	npmRunBuild := exec.Command(manager, "run", str)
	if err := RunCmd(npmRunBuild, "build", logger); err != nil {
		return err
	}

	return nil
}

// RunAdditionScript run another script before deploy.
func RunAdditionScript(logger *golog.Logger, env *ConfigEnv) error {
	npmRunBuild := exec.Command(env.AdditionalScript, env.AdditionalScriptArgs...)
	if err := RunCmd(npmRunBuild, "build", logger); err != nil {
		return err
	}

	return nil
}

// ClearTargetFolderThenMove 删除要部署的目录，移动 dist 目录至目标目录
func ClearTargetFolderThenMove(logger *golog.Logger, path string, dist string) error {
	var distStr string

	distStr = dist

	if path == "" {
		logger.Info("there is no target path, stop moving files")
		return nil
	}

	if dist == "" {
		distStr = "./dist"
	}

	clearTargetFolder := exec.Command("rm", "-rf", path)
	if err := RunCmd(clearTargetFolder, "clear/move", logger); err != nil {
		return err
	}

	move := exec.Command("mv", distStr, path)
	if err := RunCmd(move, "run", logger); err != nil {
		return err
	} else {
		logger.Info("successfully deployed")
	}

	return nil
}
