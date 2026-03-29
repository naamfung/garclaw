package main

import (
        "fmt"
        "strings"
)

var (
        SYSTEM_PROMPT = ""
)

// 默认超时配置常量（单位：秒）
const (
        DefaultShellTimeout       = 60  // shell 命令默认超时
        DefaultBlockingCmdTimeout = 5   // 可能阻塞的命令超时（交互式命令确认后执行）
        DefaultHTTPTimeout        = 120 // HTTP 请求默认超时
        DefaultPluginTimeout      = 120 // 插件 HTTP 请求默认超时
        DefaultBrowserTimeout     = 60  // 浏览器每次操作默认超时（增加以适应慢速网络）
)

const (
        SYSTEM_PROMPT_TEMPLATE_EN = `You are an AI assistant. Follow these principles:

1.  When asked about the current date or time, use the provided system time directly.
2.  When searching for time-sensitive information (like news), use the system time to construct your query.
3.  **Before calling any tool, review the entire conversation history. If the information needed to answer your current question is already present in the history (including your previous responses or tool results), answer directly without calling a tool.**

# CRITICAL: Understanding Conversation History
All messages in the conversation history are **PAST events that have already occurred**. They are records of what happened, NOT instructions to be executed again:
- Every tool_call in history has ALREADY been executed
- Every tool_result in history is the ACTUAL result of that execution
- You should NEVER re-execute any tool call from the history
- When you see a tool_result, treat it as factual information, not as a pending task

If a previous task was completed (you see tool_result with success), do NOT repeat that task. Only proceed with NEW tasks based on the user's LATEST message.

# Understanding Tool Execution Status
Every tool result has a status marker indicating the final state:
- **[COMPLETED]**: Task finished successfully. The action has been performed.
- **[OPERATION FAILED]**: Task failed due to error. The action was NOT completed.
- **[OPERATION CANCELLED BY USER]**: Task was cancelled by user. The action was STOPPED mid-execution. NEVER retry cancelled tasks - the user cancelled for a reason!
- **[OPERATION SKIPPED]**: Task was skipped because a dependency was cancelled or failed.

When you see [OPERATION CANCELLED BY USER], the user intentionally stopped that task. Do NOT retry or continue that task unless the user explicitly asks you to.

4.  Only call a tool when the necessary information is not available in the history.
5.  Provide clear and concise responses to the user.
`

        SYSTEM_PROMPT_TEMPLATE_ZH = `你是一个 AI 助手。请遵循以下原则：

1. 当被问及当前日期或时间时，直接使用系统提供的时间，不要尝试执行命令获取。
2. 当需要搜索有时效性的信息（如新闻）时，使用系统时间构造搜索关键词。
3. **在调用任何工具之前，先回顾整个对话历史。如果回答用户当前问题所需的信息已在历史中（包括你之前的回答或工具结果），请直接回答，不要调用工具。**

# 关键：理解对话历史
对话历史中的所有消息都是**已经发生的过去事件**。它们是发生过的记录，而不是需要再次执行的指令：
- 历史中的每个 tool_call 都**已经执行完毕**
- 历史中的每个 tool_result 都是那次执行的**实际结果**
- 你**绝对不要**重新执行历史中的任何工具调用
- 看到工具结果时，将其视为事实信息，而非待处理的任务

如果之前的任务已完成（你看到了成功的 tool_result），**不要重复执行该任务**。只根据用户**最新的消息**处理新任务。

# 理解工具执行状态
每个工具结果都有状态标记，表示最终状态：
- **[COMPLETED]**：任务成功完成。操作已执行。
- **[OPERATION FAILED]**：任务因错误失败。操作未完成。
- **[OPERATION CANCELLED BY USER]**：任务被用户取消。操作在执行中被停止。**绝不要重试被取消的任务**——用户取消是有原因的！
- **[OPERATION SKIPPED]**：任务被跳过，因为依赖项被取消或失败。

当你看到 [OPERATION CANCELLED BY USER] 时，说明用户有意停止了该任务。**不要重试或继续该任务**，除非用户明确要求你这样做。

4. 仅在历史中未有所需信息时才调用工具。
5. 向用户提供清晰、简洁的回答。
`
)

func init() {
        if true {
                SYSTEM_PROMPT = SYSTEM_PROMPT_TEMPLATE_ZH
        } else {
                SYSTEM_PROMPT = SYSTEM_PROMPT_TEMPLATE_EN
        }
}

// BuildSystemPromptForActor 为指定演员构建系统提示
func BuildSystemPromptForActor(actorName string, am *ActorManager, pm *RoleManager, stage *Stage) string {
        // 获取演员信息
        actor, ok := am.GetActor(actorName)
        if !ok {
                return SYSTEM_PROMPT
        }

        // 获取角色模板
        role, ok := pm.GetRole(actor.Role)
        if !ok {
                return SYSTEM_PROMPT
        }

        var prompt strings.Builder

        // 1. 角色身份和背景
        prompt.WriteString("# 角色身份\n\n")
        if actor.CharacterName != "" {
                prompt.WriteString(fmt.Sprintf("**角色名**：%s\n\n", actor.CharacterName))
        }
        if actor.CharacterBackground != "" {
                prompt.WriteString("**角色背景**：\n")
                prompt.WriteString(actor.CharacterBackground)
                prompt.WriteString("\n\n")
        }

        // 2. 角色模板内容
        prompt.WriteString(role.BuildSystemPrompt())

        // 3. 角色-技能绑定（注入角色绑定的技能提示）
        if len(role.Skills) > 0 && globalSkillManager != nil {
                prompt.WriteString("\n\n## 角色专属技能\n\n")
                prompt.WriteString("作为此角色，你已掌握以下专业技能：\n\n")

                for _, skillName := range role.Skills {
                        skill, ok := globalSkillManager.GetSkill(skillName)
                        if !ok {
                                continue
                        }
                        // 注入完整的技能提示
                        prompt.WriteString(skill.BuildSkillPrompt())
                        prompt.WriteString("\n")
                }
        }

        // 4. 可用技能索引（只显示未绑定的技能，供用户选择激活）
        if globalSkillManager != nil {
                availableSkills := buildAvailableSkillsIndex(role.Skills)
                if availableSkills != "" {
                        prompt.WriteString("\n\n")
                        prompt.WriteString(availableSkills)
                }
        }

        // 5. 手动激活的额外技能（如果有）
        if skillPrompt := GetActiveSkillPrompt(); skillPrompt != "" {
                prompt.WriteString("\n\n---\n")
                prompt.WriteString("## 额外激活技能\n\n")
                prompt.WriteString(skillPrompt)
        }

        // 6. 场景上下文（如果有）
        if stage != nil {
                stageContext := stage.BuildStageContext(am, pm)
                if stageContext != "" {
                        prompt.WriteString("\n\n")
                        prompt.WriteString(stageContext)
                }
        }

        // 7. 通用工具说明（根据角色权限过滤）
        toolSection := BuildToolSectionForRole(role)
        if toolSection != "" {
                prompt.WriteString("\n\n")
                prompt.WriteString(toolSection)
        }

        return prompt.String()
}

// buildAvailableSkillsIndex 构建可用技能索引（排除已绑定的技能）
func buildAvailableSkillsIndex(boundSkills []string) string {
        if globalSkillManager == nil {
                return ""
        }

        // 创建已绑定技能的集合
        boundSet := make(map[string]bool)
        for _, s := range boundSkills {
                boundSet[s] = true
        }

        // 收集未绑定的技能
        var available []*Skill
        for _, skill := range globalSkillManager.ListSkills() {
                if !boundSet[skill.Name] {
                        available = append(available, skill)
                }
        }

        if len(available) == 0 {
                return ""
        }

        var sb strings.Builder
        sb.WriteString("# 可用技能\n\n")
        sb.WriteString("以下技能可根据需要激活（使用 `/skill <技能名>` 激活）：\n\n")

        for _, skill := range available {
                sb.WriteString(fmt.Sprintf("- **%s** (`%s`): %s\n",
                        skill.DisplayName, skill.Name,
                        truncateString(skill.Description, 50)))
        }

        return sb.String()
}

// BuildToolSectionForRole 根据角色权限构建工具说明
func BuildToolSectionForRole(role *Role) string {
        var sb strings.Builder

        sb.WriteString("# 可用工具\n\n")
        sb.WriteString("你拥有丰富的工具来完成各种任务。工具按类别组织：\n\n")

        // 按类别组织工具
        categories := []struct {
                name        string
                tools       []struct {
                        name        string
                        description string
                }
        }{
                {"命令执行", []struct {
                        name        string
                        description string
                }{
                        {"smart_shell", "智能执行命令（自动判断同步/异步模式）"},
                        {"shell", "执行系统命令"},
                        {"shell_delayed", "异步执行长时间命令"},
                        {"spawn", "启动后台进程"},
                }},
                {"文件操作", []struct {
                        name        string
                        description string
                }{
                        {"read_file_line", "读取文件指定行"},
                        {"write_file_line", "写入文件指定行"},
                        {"read_all_lines", "读取文件所有行"},
                        {"write_all_lines", "覆盖写入文件"},
                }},
                {"文本处理", []struct {
                        name        string
                        description string
                }{
                        {"text_search", "文本搜索"},
                        {"text_grep", "正则搜索"},
                        {"text_replace", "文本替换"},
                        {"text_transform", "文本转换"},
                }},
                {"浏览器操作", []struct {
                        name        string
                        description string
                }{
                        {"browser_visit", "访问网页"},
                        {"browser_search", "搜索引擎搜索"},
                        {"browser_download", "下载文件"},
                        {"browser_screenshot", "截图"},
                        {"browser_click", "点击元素"},
                        {"browser_type", "输入文本"},
                        {"browser_scroll", "滚动页面"},
                        {"browser_execute_js", "执行 JavaScript"},
                }},
                {"记忆管理", []struct {
                        name        string
                        description string
                }{
                        {"memory_save", "保存记忆"},
                        {"memory_recall", "检索记忆"},
                        {"memory_forget", "删除记忆"},
                        {"memory_list", "列出记忆"},
                }},
                {"插件管理", []struct {
                        name        string
                        description string
                }{
                        {"plugin_list", "列出插件"},
                        {"plugin_create", "创建插件"},
                        {"plugin_load", "加载插件"},
                        {"plugin_call", "调用插件"},
                }},
                {"任务调度", []struct {
                        name        string
                        description string
                }{
                        {"cron_add", "添加定时任务"},
                        {"cron_list", "列出定时任务"},
                        {"todo", "管理待办事项"},
                }},
        }

        for _, category := range categories {
                availableTools := make([]string, 0)
                for _, tool := range category.tools {
                        if role.IsToolAllowed(tool.name) {
                                availableTools = append(availableTools, fmt.Sprintf("- **%s**：%s", tool.name, tool.description))
                        }
                }
                if len(availableTools) > 0 {
                        sb.WriteString(fmt.Sprintf("## %s\n", category.name))
                        sb.WriteString(strings.Join(availableTools, "\n"))
                        sb.WriteString("\n\n")
                }
        }

        sb.WriteString("**提示**：每个工具都有详细的参数说明。调用时系统会显示具体用法。\n")

        return sb.String()
}
