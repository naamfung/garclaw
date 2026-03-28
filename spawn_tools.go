package main

import (
        "context"
        "fmt"
        "sync"
)

// 全局会话 ID 计数器
var sessionIDCounter int
var sessionIDMutex sync.Mutex

// getCurrentSessionID 获取当前会话 ID
func getCurrentSessionID() string {
        sessionIDMutex.Lock()
        defer sessionIDMutex.Unlock()
        sessionIDCounter++
        return fmt.Sprintf("session_%d", sessionIDCounter)
}

// handleSpawn 处理 spawn 工具调用
func handleSpawn(ctx context.Context, argsMap map[string]interface{}, ch Channel) (string, error) {
        task, ok := argsMap["task"].(string)
        if !ok || task == "" {
                return "Error: 缺少任务描述 (task)", nil
        }

        maxIterations := 15 // 默认值
        if mi, ok := argsMap["max_iterations"].(float64); ok {
                maxIterations = int(mi)
        }

        // 获取或创建会话 ID
        sessionID := getCurrentSessionID()

        // 初始化子代理管理器（如果尚未初始化）
        if globalSubagentManager == nil {
                globalSubagentManager = NewSubagentManager()
                // 设置结果处理函数
                globalSubagentManager.SetResultHandler(func(task *SubagentTask) {
                        logSubagentResult(task)
                })
        }

        // 创建子代理任务
        subagentTask, err := globalSubagentManager.Spawn(task, sessionID, maxIterations)
        if err != nil {
                return fmt.Sprintf("Error: 创建子代理失败: %v", err), nil
        }

        result := fmt.Sprintf("子代理已创建:\n- 任务ID: %s\n- 任务: %s\n- 最大迭代: %d\n- 状态: running\n\n子代理将在后台执行任务。使用 spawn_check 检查进度，spawn_list 查看所有任务。",
                subagentTask.ID, task, maxIterations)

        return result, nil
}

// handleSpawnCheck 处理 spawn_check 工具调用
func handleSpawnCheck(ctx context.Context, argsMap map[string]interface{}, ch Channel) (string, error) {
        taskID, ok := argsMap["task_id"].(string)
        if !ok || taskID == "" {
                return "Error: 缺少任务ID (task_id)", nil
        }

        if globalSubagentManager == nil {
                return "Error: 子代理管理器未初始化", nil
        }

        info, err := globalSubagentManager.GetTaskInfo(taskID)
        if err != nil {
                return fmt.Sprintf("Error: %v", err), nil
        }

        result := fmt.Sprintf("子代理任务状态:\n- 任务ID: %s\n- 状态: %s\n- 迭代次数: %d/%d\n- 运行时间: %.1f 秒",
                info["task_id"], info["status"], info["iterations"], info["max_iterations"], info["runtime_seconds"])

        if info["result"] != nil && info["result"] != "" {
                result += fmt.Sprintf("\n\n结果:\n%s", info["result"])
        }

        return result, nil
}

// handleSpawnList 处理 spawn_list 工具调用
func handleSpawnList(ctx context.Context, argsMap map[string]interface{}, ch Channel) (string, error) {
        if globalSubagentManager == nil {
                return "当前没有子代理任务", nil
        }

        tasks := globalSubagentManager.List()
        if len(tasks) == 0 {
                return "当前没有子代理任务", nil
        }

        result := fmt.Sprintf("共 %d 个子代理任务:\n\n", len(tasks))
        for i, task := range tasks {
                task.mu.RLock()
                result += fmt.Sprintf("%d. [%s] %s\n   状态: %s | 迭代: %d/%d\n\n",
                        i+1, task.ID, truncateString(task.Task, 50), task.Status, task.Iterations, task.MaxIterations)
                task.mu.RUnlock()
        }

        return result, nil
}

// handleSpawnCancel 处理 spawn_cancel 工具调用
func handleSpawnCancel(ctx context.Context, argsMap map[string]interface{}, ch Channel) (string, error) {
        taskID, ok := argsMap["task_id"].(string)
        if !ok || taskID == "" {
                return "Error: 缺少任务ID (task_id)", nil
        }

        if globalSubagentManager == nil {
                return "Error: 子代理管理器未初始化", nil
        }

        err := globalSubagentManager.Cancel(taskID)
        if err != nil {
                return fmt.Sprintf("Error: %v", err), nil
        }

        return fmt.Sprintf("子代理任务 %s 已取消", taskID), nil
}

// logSubagentResult 记录子代理结果
func logSubagentResult(task *SubagentTask) {
        task.mu.RLock()
        defer task.mu.RUnlock()

        logMessage := fmt.Sprintf("[Subagent] Task %s completed - Status: %s, Iterations: %d",
                task.ID, task.Status, task.Iterations)

        if task.Result != "" {
                logMessage += fmt.Sprintf(", Result length: %d chars", len(task.Result))
        }

        // 这里可以添加通知逻辑，比如通过消息总线通知主代理
        // 目前只是打印日志
        fmt.Println(logMessage)
}
