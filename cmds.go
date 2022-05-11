package main

import (
	"bufio"
	"fmt"
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

	//defer func() {
	//	if err := stdout.Close(); err != nil {
	//		logger.Error("close cmd failed:", err)
	//	}
	//}()

	if err := c.Start(); err != nil {
		return err
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Split(bufio.ScanLines)
Loop:
	for scanner.Scan() {
		m := scanner.Text()
		fmt.Println(m)
		if strings.Contains(m, "exit status") {
			if err := c.Process.Release(); err != nil {
				panic(err)
			}
			released = true
			logger.Info("process released")

			break Loop
		}
	}

	if !released {
		if err := c.Wait(); err != nil {
			return fmt.Errorf("command wait error:%w", err)
		}
	}

	return nil
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
func RunBuild(logger *golog.Logger, script string) error {
	var str string

	str = script
	if script == "" {
		str = "build"
	}

	npmRunBuild := exec.Command("npm", "run", str)
	if err := RunCmd(npmRunBuild, "build", logger); err != nil {
		return err
	}

	return nil
}

// ClearTargetFolderThenMove 删除要部署的目录，移动 dist 目录至目标目录
func ClearTargetFolderThenMove(logger *golog.Logger, path string, dist string) error {
	var distStr string

	distStr = dist

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
	}

	return nil
}
