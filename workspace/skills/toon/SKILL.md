---
name: TOON格式处理
description: 用于读写与处理TOON格式文档的技能，支持Token-Oriented Object Notation格式的解析与生成
invocation: toon
---

# TOON格式处理技能

## 何为TOON

TOON（Token-Oriented Object Notation）是一种紧凑、机器与人类可读的JSON数据模型编码格式，旨在最小化token数量并使结构易于模型理解。它是为LLM输入设计的，可以作为现有JSON的无损替代品。

TOON结合了YAML的缩进式结构（用于嵌套对象）与CSV风格的表格布局（用于统一数组）。TOON的优势在于处理统一对象数组时，实现类似CSV的紧凑性，同时添加明确的结构以帮助LLMs可靠地解析与验证数据。

## 核心功能

### 1. 读取TOON文件

**功能**：读取并解析TOON格式的文件，转换为内部数据结构。

**使用场景**：当需要加载TOON格式的配置文件、数据文件或任务文件时使用。

**使用方法**：
1. 使用文件读取工具（如read_all_lines）读取TOON文件内容
2. 分析TOON格式内容，解析为内部数据结构

**示例**：
```
# 步骤1: 读取TOON文件内容
# 使用read_all_lines工具读取文件

# 步骤2: 解析TOON内容
# 示例TOON内容
users[2]{id,name,role}:
  1,Alice,admin
  2,Bob,user

# 解析后的数据结构
{
  "users": [
    { "id": 1, "name": "Alice", "role": "admin" },
    { "id": 2, "name": "Bob", "role": "user" }
  ]
}
```

### 2. 写入TOON文件

**功能**：将内部数据结构转换为TOON格式并写入文件。

**使用场景**：当需要保存数据为TOON格式，以便后续处理或作为LLM输入时使用。

**使用方法**：
1. 首先将数据结构转换为TOON格式的字符串
2. 使用文件写入工具（如write_all_lines）将TOON字符串写入文件

**示例**：
```
# 步骤1: 构建TOON格式字符串
toon_content = """
users[2]{id,name,role}:
  1,Alice,admin
  2,Bob,user
"""

# 步骤2: 写入文件
# 使用write_all_lines工具将内容写入文件
```

### 3. TOON格式转换

**功能**：在JSON与TOON格式之间进行转换。

**使用场景**：当需要在现有JSON数据与TOON格式之间转换时使用。

**使用方法**：
1. JSON转TOON：分析JSON结构，根据TOON语法规则构建相应的TOON格式
2. TOON转JSON：解析TOON格式，构建对应的JSON结构

**示例**：

**JSON转TOON**：
```
# 原始JSON
{
  "users": [
    { "id": 1, "name": "Alice", "role": "admin" },
    { "id": 2, "name": "Bob", "role": "user" }
  ]
}

# 转换为TOON
users[2]{id,name,role}:
  1,Alice,admin
  2,Bob,user
```

**TOON转JSON**：
```
# 原始TOON
users[2]{id,name,role}:
  1,Alice,admin
  2,Bob,user

# 转换为JSON
{
  "users": [
    { "id": 1, "name": "Alice", "role": "admin" },
    { "id": 2, "name": "Bob", "role": "user" }
  ]
}
```

## TOON语法规则

### 基本结构

1. **对象**：使用缩进表示嵌套，键值对用冒号分隔
   ```yaml
   id: 1
   name: Ada
   ```

2. **嵌套对象**：
   ```yaml
   user:
     id: 1
     name: Ada
   ```

3. **原始数组**：使用`[N]`表示数组长度，元素用逗号分隔
   ```yaml
   tags[3]: foo,bar,baz
   ```

4. **表格数组**：使用`[N]{field1,field2}`声明数组长度与字段名，每行是一个逗号分隔的值列表
   ```yaml
   items[2]{id,qty}:
     1,5
     2,3
   ```

5. **混合/非均匀数组**：使用`[N]`表示数组长度，每个元素用`-`标记
   ```yaml
   items[3]:
     - 1
     - a: 1
     - x
   ```

### 引用规则

字符串必须在以下情况加引号：
- 为空字符串 (`""`)
- 有前导或尾随空格
- 等于 `true`、`false` 或 `null`（区分大小写）
- 看起来像数字（例如，`"42"`、`"-3.14"`）
- 包含特殊字符：`:`, `"`, `\`, `[`, `]`, `{`, `}`, 换行符, 制表符, 回车符
- 包含活动分隔符（默认为逗号）
- 等于 `"-"` 或以 `"-"` 开头后跟任何字符

### 转义序列

在带引号的字符串中，只有五种转义序列有效：
- 反斜杠 (`\`) → `\\`
- 双引号 (`"`) → `\"`
- 换行符 → `\n`
- 回车符 → `\r`
- 制表符 → `\t`

## 最佳实践

1. **何时使用TOON**：
   - 处理统一对象数组时（TOON的最佳使用场景）
   - 需要减少token数量的LLM输入
   - 需要明确结构标记以提高解析可靠性时

2. **何时不使用TOON**：
   - 深度嵌套或非均匀结构
   - 半均匀数组（表格 eligibility ≈ 40–60%）
   - 纯表格数据（CSV更小）
   - 延迟关键应用（需在特定环境中基准测试）

3. **性能考虑**：
   - TOON在处理统一对象数组时，效率提升与行数与字段数线性相关
   - 简单对象与原始数组显示一致的字节减少
   - 嵌套对象受益于减少的开销，但效率随深度降低
   - 数组的数组是TOON效率低于JSON的唯一结构

## 实现建议

1. **依赖选择**：
   - 考虑使用官方TOON库（如TypeScript、Python、Go、Rust、.NET等）
   - 或实现轻量级解析器，专注于核心TOON功能

2. **错误处理**：
   - 实现健壮的错误处理，特别是在解析TOON文件时
   - 提供清晰的错误信息，帮助定位格式问题

3. **测试策略**：
   - 测试各种TOON结构的解析与生成
   - 验证JSON与TOON之间的无损转换
   - 测试边界情况与特殊字符处理

## 示例应用

### 配置文件管理

使用TOON格式存储应用配置，减少配置文件大小并提高可读性。

### 数据交换

在系统组件之间使用TOON格式交换数据，减少网络传输量。

### LLM提示优化

将结构化数据转换为TOON格式，减少LLM提示中的token数量，降低成本并提高解析可靠性。

### 任务定义

使用TOON格式定义任务与工作流，如CRON任务配置等。