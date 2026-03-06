package main

// 工具定义
func getTools(apiType string) interface{} {
	switch apiType {
	case "openai":
		// DeepSeek与OpenAI使用tools格式，包含type: "function"
		return []map[string]interface{}{
			{
				"type": "function",
				"function": map[string]interface{}{
					"name":        "shell",
					"description": "Execute a shell command to perform tasks like listing files, creating directories, or running programs. Use this when the user asks to execute any command or when you need to interact with the system. Always use this tool when the user asks to list files, check directory contents, or run any system command.",
					"parameters": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"command": map[string]interface{}{
								"type":        "string",
								"description": "The shell command to execute. For example, use 'ls' or 'ls -la' (Unix/Linux) to list files, 'mkdir test' to create a directory, 'echo hello' to print text.",
							},
						},
						"required":             []string{"command"},
						"additionalProperties": false,
					},
				},
			},
			{
				"type": "function",
				"function": map[string]interface{}{
					"name":        "read_file_line",
					"description": "Read a specific line from a file. Use this when you need to read a particular line from a file without reading the entire file.",
					"parameters": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"filename": map[string]interface{}{
								"type":        "string",
								"description": "The path to the file to read.",
							},
							"line_num": map[string]interface{}{
								"type":        "integer",
								"description": "The line number to read (starting from 1).",
							},
						},
						"required":             []string{"filename", "line_num"},
						"additionalProperties": false,
					},
				},
			},
			{
				"type": "function",
				"function": map[string]interface{}{
					"name":        "write_file_line",
					"description": "Write content to a specific line in a file. If the line number is beyond the current file length, the file will be extended with empty lines.",
					"parameters": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"filename": map[string]interface{}{
								"type":        "string",
								"description": "The path to the file to write to.",
							},
							"line_num": map[string]interface{}{
								"type":        "integer",
								"description": "The line number to write to (starting from 1).",
							},
							"content": map[string]interface{}{
								"type":        "string",
								"description": "The content to write to the specified line.",
							},
						},
						"required":             []string{"filename", "line_num", "content"},
						"additionalProperties": false,
					},
				},
			},
			{
				"type": "function",
				"function": map[string]interface{}{
					"name":        "read_all_lines",
					"description": "Read all lines from a file and return them as a list of strings.",
					"parameters": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"filename": map[string]interface{}{
								"type":        "string",
								"description": "The path to the file to read.",
							},
						},
						"required":             []string{"filename"},
						"additionalProperties": false,
					},
				},
			},
			{
				"type": "function",
				"function": map[string]interface{}{
					"name":        "write_all_lines",
					"description": "Write all lines to a file, overwriting the existing content.",
					"parameters": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"filename": map[string]interface{}{
								"type":        "string",
								"description": "The path to the file to write to.",
							},
							"lines": map[string]interface{}{
								"type": "array",
								"items": map[string]interface{}{
									"type": "string",
								},
								"description": "The list of lines to write to the file.",
							},
						},
						"required":             []string{"filename", "lines"},
						"additionalProperties": false,
					},
				},
			},
			{
				"type": "function",
				"function": map[string]interface{}{
					"name":        "search",
					"description": "Search for a keyword using Baidu search engine.",
					"parameters": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"keyword": map[string]interface{}{
								"type":        "string",
								"description": "The keyword to search for.",
							},
						},
						"required":             []string{"keyword"},
						"additionalProperties": false,
					},
				},
			},
			{
				"type": "function",
				"function": map[string]interface{}{
					"name":        "visit",
					"description": "Visit a URL and retrieve its content.",
					"parameters": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"url": map[string]interface{}{
								"type":        "string",
								"description": "The URL to visit.",
							},
						},
						"required":             []string{"url"},
						"additionalProperties": false,
					},
				},
			},
			{
				"type": "function",
				"function": map[string]interface{}{
					"name":        "download",
					"description": "Download a web page or file from a given URL.",
					"parameters": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"url": map[string]interface{}{
								"type":        "string",
								"description": "The URL to download.",
							},
						},
						"required":             []string{"url"},
						"additionalProperties": false,
					},
				},
			},

			{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "todo",
				"description": "Update task list. Track progress on multi-step tasks.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"items": map[string]interface{}{
							"type": "array",
							"items": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"id": map[string]interface{}{
										"type":        "string",
										"description": "Task ID.",
									},
									"text": map[string]interface{}{
										"type":        "string",
										"description": "Task description.",
									},
									"status": map[string]interface{}{
										"type":        "string",
										"enum":        []string{"pending", "in_progress", "completed"},
										"description": "Task status: pending, in_progress, or completed.",
									},
								},
								"required": []string{"id", "text", "status"},
							},
							"description": "List of tasks.",
						},
					},
					"required":             []string{"items"},
					"additionalProperties": false,
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "memory_write",
				"description": "Write content to memory. Use this to store information that should be remembered for future interactions.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"content": map[string]interface{}{
							"type":        "string",
							"description": "The content to write to memory.",
						},
					},
					"required":             []string{"content"},
					"additionalProperties": false,
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "memory_search",
				"description": "Search memory for content. Use this to retrieve information that was previously stored in memory.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query": map[string]interface{}{
							"type":        "string",
							"description": "The search query to find in memory.",
						},
					},
					"required":             []string{"query"},
					"additionalProperties": false,
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "calculate",
				"description": "Perform arithmetic operations on two numbers. Use this when the user asks to calculate or compute mathematical expressions.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"operation": map[string]interface{}{
							"type":        "string",
							"description": "The arithmetic operation to perform: add, subtract, multiply, divide.",
						},
						"num1": map[string]interface{}{
							"type":        "number",
							"description": "The first number for the operation.",
						},
						"num2": map[string]interface{}{
							"type":        "number",
							"description": "The second number for the operation.",
						},
					},
					"required":             []string{"operation", "num1", "num2"},
					"additionalProperties": false,
				},
			},
		},
	}

default:
		// Anthropic与Ollama使用tools格式
		return []map[string]interface{}{
			{
				"name":        "shell",
				"description": "Execute a shell command to perform tasks like listing files, creating directories, or running programs. Use this when the user asks to execute any command or when you need to interact with the system. Always use this tool when the user asks to list files, check directory contents, or run any system command.",
				"input_schema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"command": map[string]interface{}{
							"type":        "string",
							"description": "The shell command to execute. For example, use 'ls' or 'ls -la' (Unix/Linux) to list files, 'mkdir test' to create a directory, 'echo hello' to print text.",
						},
					},
					"required":             []string{"command"},
					"additionalProperties": false,
				},
			},
			{
				"name":        "read_file_line",
				"description": "Read a specific line from a file. Use this when you need to read a particular line from a file without reading the entire file.",
				"input_schema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"filename": map[string]interface{}{
							"type":        "string",
							"description": "The path to the file to read.",
						},
						"line_num": map[string]interface{}{
							"type":        "integer",
							"description": "The line number to read (starting from 1).",
						},
					},
					"required":             []string{"filename", "line_num"},
					"additionalProperties": false,
				},
			},
			{
				"name":        "write_file_line",
				"description": "Write content to a specific line in a file. If the line number is beyond the current file length, the file will be extended with empty lines.",
				"input_schema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"filename": map[string]interface{}{
							"type":        "string",
							"description": "The path to the file to write to.",
						},
						"line_num": map[string]interface{}{
							"type":        "integer",
							"description": "The line number to write to (starting from 1).",
						},
						"content": map[string]interface{}{
							"type":        "string",
							"description": "The content to write to the specified line.",
						},
					},
					"required":             []string{"filename", "line_num", "content"},
					"additionalProperties": false,
				},
			},
			{
				"name":        "read_all_lines",
				"description": "Read all lines from a file and return them as a list of strings.",
				"input_schema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"filename": map[string]interface{}{
							"type":        "string",
							"description": "The path to the file to read.",
						},
					},
					"required":             []string{"filename"},
					"additionalProperties": false,
				},
			},
			{
				"name":        "write_all_lines",
				"description": "Write all lines to a file, overwriting the existing content.",
				"input_schema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"filename": map[string]interface{}{
							"type":        "string",
							"description": "The path to the file to write to.",
						},
						"lines": map[string]interface{}{
							"type": "array",
							"items": map[string]interface{}{
								"type": "string",
							},
							"description": "The list of lines to write to the file.",
						},
					},
					"required":             []string{"filename", "lines"},
					"additionalProperties": false,
				},
			},
			{
				"name":        "search",
				"description": "Search for a keyword using Baidu search engine.",
				"input_schema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"keyword": map[string]interface{}{
							"type":        "string",
							"description": "The keyword to search for.",
						},
					},
					"required":             []string{"keyword"},
					"additionalProperties": false,
				},
			},
			{
				"name":        "visit",
				"description": "Visit a URL and retrieve its content.",
				"input_schema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"url": map[string]interface{}{
							"type":        "string",
							"description": "The URL to visit.",
						},
					},
					"required":             []string{"url"},
					"additionalProperties": false,
				},
			},
			{
				"name":        "download",
				"description": "Download a web page or file from a given URL.",
				"input_schema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"url": map[string]interface{}{
							"type":        "string",
							"description": "The URL to download.",
						},
					},
					"required":             []string{"url"},
					"additionalProperties": false,
				},
			},

			{
			"name":        "todo",
			"description": "Update task list. Track progress on multi-step tasks.",
			"input_schema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"items": map[string]interface{}{
						"type": "array",
						"items": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"id": map[string]interface{}{
									"type":        "string",
									"description": "Task ID.",
								},
								"text": map[string]interface{}{
									"type":        "string",
									"description": "Task description.",
								},
								"status": map[string]interface{}{
									"type":        "string",
									"enum":        []string{"pending", "in_progress", "completed"},
									"description": "Task status: pending, in_progress, or completed.",
								},
							},
							"required": []string{"id", "text", "status"},
						},
						"description": "List of tasks.",
					},
				},
				"required":             []string{"items"},
				"additionalProperties": false,
			},
		},
		{
			"name":        "memory_write",
			"description": "Write content to memory. Use this to store information that should be remembered for future interactions.",
			"input_schema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"content": map[string]interface{}{
						"type":        "string",
						"description": "The content to write to memory.",
					},
				},
				"required":             []string{"content"},
				"additionalProperties": false,
			},
		},
		{
			"name":        "memory_search",
			"description": "Search memory for content. Use this to retrieve information that was previously stored in memory.",
			"input_schema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "The search query to find in memory.",
					},
				},
				"required":             []string{"query"},
				"additionalProperties": false,
			},
		},
		{
			"name":        "calculate",
			"description": "Perform arithmetic operations on two numbers. Use this when the user asks to calculate or compute mathematical expressions.",
			"input_schema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"operation": map[string]interface{}{
						"type":        "string",
						"description": "The arithmetic operation to perform: add, subtract, multiply, divide.",
					},
					"num1": map[string]interface{}{
						"type":        "number",
						"description": "The first number for the operation.",
					},
					"num2": map[string]interface{}{
						"type":        "number",
						"description": "The second number for the operation.",
					},
				},
				"required":             []string{"operation", "num1", "num2"},
				"additionalProperties": false,
			},
		},
	}
	}
}
