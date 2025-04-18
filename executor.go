package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"
)

// ExecutionResult 表示代码执行的结果
type ExecutionResult struct {
	Success bool   `json:"success"`
	Output  string `json:"output"`
	Error   string `json:"error"`
}

// CodeExecutor 是代码执行器的主要结构体
type CodeExecutor struct {
	timeout         time.Duration
	maxWorkers      int
	workerPool      chan struct{}
	nodejsAvailable bool
	mu              sync.Mutex
}

// NewCodeExecutor 创建一个新的代码执行器实例
func NewCodeExecutor(timeout int, maxWorkers int) *CodeExecutor {
	executor := &CodeExecutor{
		timeout:         time.Duration(timeout) * time.Second,
		maxWorkers:      maxWorkers,
		workerPool:      make(chan struct{}, maxWorkers),
		nodejsAvailable: checkNodeJSAvailable(),
	}
	return executor
}

// checkNodeJSAvailable 检查Node.js是否可用
func checkNodeJSAvailable() bool {
	cmd := exec.Command("node", "--version")
	err := cmd.Run()
	return err == nil
}

// runPythonCode 在进程中执行Python代码
func runPythonCode(code string) ExecutionResult {
	var stdout, stderr bytes.Buffer

	// 创建临时文件
	tmpFile, err := os.CreateTemp("", "python-*.py")
	if err != nil {
		return ExecutionResult{
			Success: false,
			Error:   fmt.Sprintf("创建临时文件失败: %v", err),
		}
	}
	defer os.Remove(tmpFile.Name())

	// 写入代码到临时文件
	if _, err := tmpFile.WriteString(code); err != nil {
		return ExecutionResult{
			Success: false,
			Error:   fmt.Sprintf("写入代码失败: %v", err),
		}
	}
	tmpFile.Close()

	// 执行Python代码
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "python", tmpFile.Name())
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		return ExecutionResult{
			Success: false,
			Output:  stdout.String(),
			Error:   stderr.String(),
		}
	}

	return ExecutionResult{
		Success: true,
		Output:  stdout.String(),
		Error:   "",
	}
}

// runNodeJSCode 在进程中执行Node.js代码
func runNodeJSCode(code string) ExecutionResult {
	var stdout, stderr bytes.Buffer

	// 创建临时文件
	tmpFile, err := os.CreateTemp("", "nodejs-*.js")
	if err != nil {
		return ExecutionResult{
			Success: false,
			Error:   fmt.Sprintf("创建临时文件失败: %v", err),
		}
	}
	defer os.Remove(tmpFile.Name())

	// 写入代码到临时文件
	if _, err := tmpFile.WriteString(code); err != nil {
		return ExecutionResult{
			Success: false,
			Error:   fmt.Sprintf("写入代码失败: %v", err),
		}
	}
	tmpFile.Close()

	// 执行Node.js代码
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "node", tmpFile.Name())
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		return ExecutionResult{
			Success: false,
			Output:  stdout.String(),
			Error:   stderr.String(),
		}
	}

	return ExecutionResult{
		Success: true,
		Output:  stdout.String(),
		Error:   "",
	}
}

// Execute 执行代码
func (e *CodeExecutor) Execute(code string, language string) ExecutionResult {
	// 获取工作池令牌
	e.workerPool <- struct{}{}
	defer func() { <-e.workerPool }()

	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	resultChan := make(chan ExecutionResult, 1)

	go func() {
		var result ExecutionResult
		switch language {
		case "python3":
			result = runPythonCode(code)
		case "nodejs":
			if !e.nodejsAvailable {
				result = ExecutionResult{
					Success: false,
					Error:   "Node.js未安装或不可用",
				}
			} else {
				result = runNodeJSCode(code)
			}
		default:
			result = ExecutionResult{
				Success: false,
				Error:   fmt.Sprintf("不支持的语言: %s", language),
			}
		}
		resultChan <- result
	}()

	select {
	case result := <-resultChan:
		return result
	case <-ctx.Done():
		return ExecutionResult{
			Success: false,
			Error:   fmt.Sprintf("代码执行超时 (>%d秒)", int(e.timeout.Seconds())),
		}
	}
}

// Shutdown 关闭执行器
func (e *CodeExecutor) Shutdown() {
	// 等待所有工作完成
	for i := 0; i < e.maxWorkers; i++ {
		e.workerPool <- struct{}{}
	}
}
