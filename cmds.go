package main

import (
	"bufio"
	"errors"
	"io"
	"os/exec"
	"strings"
	"sync"

	"github.com/kataras/golog"
)

// RunCmd cmd 进程通用方法
func RunCmd(c *exec.Cmd, cType string, logger *golog.Logger) error {
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

	var wg sync.WaitGroup
	releasedCh := make(chan bool, 1)

	// 并发读取 stdout
	wg.Add(1)
	go func() {
		defer wg.Done()
		if r := doScan(c, stdout, logger, false); r {
			select {
			case releasedCh <- true:
			default:
			}
		}
	}()

	// 并发读取 stderr
	wg.Add(1)
	go func() {
		defer wg.Done()
		if r := doScan(c, stdErrOut, logger, true); r {
			select {
			case releasedCh <- true:
			default:
			}
		}
	}()

	// 等待命令完成
	cmdDone := make(chan error, 1)
	go func() {
		cmdDone <- c.Wait()
	}()

	// 检查是否有进程被释放或命令完成
	select {
	case <-releasedCh:
		// 进程被释放，等待所有输出读取完成后再返回
		wg.Wait()
		return errors.New("command error")
	case err := <-cmdDone:
		// 命令完成，等待所有输出读取完成
		// 确保所有 stdout 和 stderr 的数据都被读取完毕
		wg.Wait()
		
		if err != nil {
			logger.Error("command wait error:", err)
			//return err
		}
	}

	return nil
}

func doScan(c *exec.Cmd, stdout io.ReadCloser, logger *golog.Logger, isError bool) bool {
	released := false
	scanner := bufio.NewScanner(stdout)
	scanner.Split(bufio.ScanLines)
Loop:
	for {
		// 使用 Scan() 读取，如果返回 false 且没有错误，说明读取完毕
		if !scanner.Scan() {
			break Loop
		}
		
		m := scanner.Text()
		// 根据 isError 参数区分日志级别
		if isError {
			logger.Error(m)
		} else {
			logger.Info(m)
		}
		if strings.Contains(m, "exit code") {
			if err := c.Process.Release(); err != nil {
				panic(err)
			}
			released = true
			logger.Info("process released")
			break Loop
		}
	}
	
	// 处理扫描错误
	if err := scanner.Err(); err != nil {
		// EOF 和文件关闭错误是正常的，表示读取完毕
		errStr := err.Error()
		if err == io.EOF ||
		   strings.Contains(errStr, "file already closed") || 
		   strings.Contains(errStr, "read |0: file already closed") ||
		   strings.Contains(errStr, "broken pipe") ||
		   strings.Contains(errStr, "use of closed file") {
			// 这些是正常的文件关闭情况，不需要记录
			return released
		}
		// 其他错误才记录
		logger.Error("scanner error:", err)
	}
	
	
	return released
}

// GitClean run cmd git clean.
func GitClean(logger *golog.Logger) error {
	gitPullCmd := exec.Command("git", "clean", "-f")
	if err := RunCmd(gitPullCmd, "clean", logger); err != nil {
		return err
	}

	gitResetCmd := exec.Command("git", "reset", "--hard")
	if err := RunCmd(gitResetCmd, "reset", logger); err != nil {
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
	extraCmd := exec.Command(env.AdditionalScript, env.AdditionalScriptArgs...)
	if err := RunCmd(extraCmd, "build", logger); err != nil {
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
