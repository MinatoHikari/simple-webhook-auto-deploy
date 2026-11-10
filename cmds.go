package main

import (
	"bufio"
	"errors"
	"io"
	"os/exec"
	"strings"
	"sync"
)

// RunCmd 执行命令,实时把 stdout/stderr 写入日志 sink。
// 命令以非零退出码结束时返回错误,调用方必须中断后续部署步骤。
func RunCmd(c *exec.Cmd, sink *LogSink, stage, deployment string) error {
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
		if r := doScan(c, stdout, sink, stage, deployment, false); r {
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
		if r := doScan(c, stdErrOut, sink, stage, deployment, true); r {
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

	select {
	case <-releasedCh:
		// 输出中检测到 "exit code",进程已被释放,等待输出读完后返回错误
		wg.Wait()
		return errors.New("command error")
	case err := <-cmdDone:
		// 命令结束,等全部输出读完后向上传递结果
		wg.Wait()
		if err != nil {
			sink.Log("error", stage, deployment, "命令退出异常: "+err.Error())
			return err
		}
	}

	return nil
}

// doScan 逐行读取命令输出并写入日志 sink,stderr 记为 error 级别。
func doScan(c *exec.Cmd, stdout io.ReadCloser, sink *LogSink, stage, deployment string, isError bool) bool {
	released := false
	cls := &stderrClassifier{}
	scanner := bufio.NewScanner(stdout)
	scanner.Split(bufio.ScanLines)
	// 构建工具可能输出超长单行(如 sourcemap),调大缓冲上限
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
Loop:
	for {
		if !scanner.Scan() {
			break Loop
		}

		m := scanner.Text()
		if m == "" {
			continue
		}
		if isError {
			sink.Log(cls.Level(m), stage, deployment, m)
		} else {
			sink.Log("info", stage, deployment, m)
		}
		// 某些命令卡死时会持续输出 "exit code" 而不退出,此时强制释放进程
		if strings.Contains(m, "exit code") {
			if err := c.Process.Release(); err != nil {
				panic(err)
			}
			released = true
			sink.Log("info", stage, deployment, "process released")
			break Loop
		}
	}

	if err := scanner.Err(); err != nil {
		errStr := err.Error()
		if err == io.EOF ||
			strings.Contains(errStr, "file already closed") ||
			strings.Contains(errStr, "read |0: file already closed") ||
			strings.Contains(errStr, "broken pipe") ||
			strings.Contains(errStr, "use of closed file") {
			// 进程被释放后的正常读错误,忽略
			return released
		}
		sink.Log("error", stage, deployment, "读取命令输出失败: "+errStr)
	}

	return released
}

// stderrClassifier 带状态的 stderr 行级别分类器。
// 构建工具的警告/诊断是"多行块":只有首行有标记(如 "(!) Some chunks are larger..."
// 或 "[PLUGIN_TIMINGS] ..."),后续行("- plugin xxx transform"、"See https://...")
// 是纯文本,逐行独立分类会把整块误标成 error。分类器记录当前块的级别,
// 让续行继承;块的级别持续到被新块覆盖或遇到横幅/成功标记为止。
//
// 部署成败由命令退出码权威决定,行级分类只影响展示:
// 无法识别的行默认降为 warn,宁可黄色也不误报红。
type stderrClassifier struct {
	block string // 当前多行块的级别,空 = 无活动块
}

// Level 返回一行 stderr 内容的日志级别。
func (sc *stderrClassifier) Level(line string) string {
	t := strings.TrimSpace(line)

	// 包管理器的脚本横幅,如 "> pkg@1.0.0 build" / "> cross-env ..." / "$ cross-env ..."
	if strings.HasPrefix(t, ">") || strings.HasPrefix(t, "$") {
		sc.block = ""
		return "info"
	}

	lt := strings.ToLower(t)

	// 明确的错误标识(vue-tsc 的 "error TS2322"、Node 的 "Error:" 等),开块
	if strings.Contains(lt, "error") || strings.Contains(lt, "failed") || strings.Contains(lt, "fatal") {
		sc.block = "error"
		return "error"
	}

	// 警告/诊断块首行,任一命中即开 warn 块:
	// - npm warn / pnpm warn / "for this warning" / deprecated
	// - Vite 经典警告前缀 "(!)"
	// - ⚠ 前缀
	// - 方括号诊断标签:[PLUGIN_TIMINGS] / [INEFFECTIVE_DYNAMIC_IMPORT] / [plugin builtin:vite-reporter]
	//   (含 error 字样的标签已被上面的错误规则优先捕获)
	if strings.Contains(lt, "warn") || strings.Contains(lt, "deprecated") ||
		strings.HasPrefix(t, "(!)") || strings.HasPrefix(t, "⚠") || strings.HasPrefix(t, "[") {
		sc.block = "warn"
		return "warn"
	}

	// 成功标记(部分工具打到 stderr):✓ / ✨ / built in / done in
	if strings.HasPrefix(t, "✓") || strings.HasPrefix(t, "✨") ||
		strings.Contains(lt, "built in") || strings.Contains(lt, "done in") {
		sc.block = ""
		return "info"
	}

	// 多行块的续行:继承当前块级别
	// (空行不清块——vue-tsc 错误块的代码框里有空行)
	if sc.block != "" {
		return sc.block
	}

	// 无法识别的独立行,默认 warn
	return "warn"
}

// GitClean 清理工作区:git clean -f + git reset --hard。
func GitClean(dir string, sink *LogSink, deployment string) error {
	gitCleanCmd := exec.Command("git", "clean", "-f")
	gitCleanCmd.Dir = dir
	if err := RunCmd(gitCleanCmd, sink, "clean", deployment); err != nil {
		return err
	}

	gitResetCmd := exec.Command("git", "reset", "--hard")
	gitResetCmd.Dir = dir
	if err := RunCmd(gitResetCmd, sink, "clean", deployment); err != nil {
		return err
	}

	return nil
}

// GitPull 拉取最新代码。
func GitPull(dir string, sink *LogSink, deployment string) error {
	gitPullCmd := exec.Command("git", "pull")
	gitPullCmd.Dir = dir
	if err := RunCmd(gitPullCmd, sink, "pull", deployment); err != nil {
		return err
	}

	return nil
}

// GitCheckout 切换分支。
func GitCheckout(dir, branch string, sink *LogSink, deployment string) error {
	gitCheckoutCmd := exec.Command("git", "checkout", branch)
	gitCheckoutCmd.Dir = dir
	if err := RunCmd(gitCheckoutCmd, sink, "checkout", deployment); err != nil {
		return err
	}

	return nil
}

// RunBuild 执行包管理器构建命令,默认 npm run build。
func RunBuild(dir, script, manager string, sink *LogSink, deployment string) error {
	if script == "" {
		script = "build"
	}
	if manager == "" {
		manager = "npm"
	}
	buildCmd := exec.Command(manager, "run", script)
	buildCmd.Dir = dir
	if err := RunCmd(buildCmd, sink, "build", deployment); err != nil {
		return err
	}

	return nil
}

// RunAdditionScript 构建前执行附加脚本。
func RunAdditionScript(dir, script string, args []string, sink *LogSink, deployment string) error {
	extraCmd := exec.Command(script, args...)
	extraCmd.Dir = dir
	if err := RunCmd(extraCmd, sink, "script", deployment); err != nil {
		return err
	}

	return nil
}

// ClearTargetFolderThenMove 删除目标目录,把构建产物移动过去。
// output 为空时跳过(只构建不发布)。
func ClearTargetFolderThenMove(dir, path, dist string, sink *LogSink, deployment string) error {
	if path == "" {
		sink.Log("info", "deploy", deployment, "未配置 output,跳过产物发布")
		return nil
	}

	if dist == "" {
		dist = "./dist"
	}

	clearTargetFolder := exec.Command("rm", "-rf", path)
	clearTargetFolder.Dir = dir
	if err := RunCmd(clearTargetFolder, sink, "deploy", deployment); err != nil {
		return err
	}

	move := exec.Command("mv", dist, path)
	move.Dir = dir
	if err := RunCmd(move, sink, "deploy", deployment); err != nil {
		return err
	}

	sink.Log("info", "deploy", deployment, "successfully deployed")
	return nil
}
