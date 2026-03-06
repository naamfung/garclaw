package main

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	MAX_SKILLS = 150
	MAX_SKILLS_PROMPT = 30000
)

// Skill 技能结构
type Skill struct {
	Name        string
	Description string
	Invocation  string
	Body        string
	Path        string
}

// SkillsManager 管理技能的发现和注入
type SkillsManager struct {
	workspaceDir string
	skills      []Skill
}

// NewSkillsManager 创建一个新的 SkillsManager 实例
func NewSkillsManager(workspaceDir string) *SkillsManager {
	return &SkillsManager{
		workspaceDir: workspaceDir,
		skills:      []Skill{},
	}
}

// _parseFrontmatter 解析简单的 YAML frontmatter
func (sm *SkillsManager) _parseFrontmatter(text string) map[string]string {
	meta := make(map[string]string)
	if !strings.HasPrefix(text, "---") {
		return meta
	}
	parts := strings.SplitN(text, "---", 3)
	if len(parts) < 3 {
		return meta
	}
	for _, line := range strings.Split(parts[1], "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.Contains(line, ":") {
			continue
		}
		keyValue := strings.SplitN(line, ":", 2)
		if len(keyValue) != 2 {
			continue
		}
		key := strings.TrimSpace(keyValue[0])
		value := strings.TrimSpace(keyValue[1])
		// 移除引号
		value = strings.Trim(value, `"'`)
		meta[key] = value
	}
	return meta
}

// _scanDir 扫描目录中的技能
func (sm *SkillsManager) _scanDir(base string) []Skill {
	var found []Skill
	if _, err := os.Stat(base); os.IsNotExist(err) {
		return found
	}
	files, err := os.ReadDir(base)
	if err != nil {
		return found
	}
	for _, file := range files {
		if !file.IsDir() {
			continue
		}
		skillPath := filepath.Join(base, file.Name())
		skillMdPath := filepath.Join(skillPath, "SKILL.md")
		if _, err := os.Stat(skillMdPath); os.IsNotExist(err) {
			continue
		}
		data, err := os.ReadFile(skillMdPath)
		if err != nil {
			continue
		}
		content := string(data)
		meta := sm._parseFrontmatter(content)
		if meta["name"] == "" {
			continue
		}
		body := ""
		if strings.Contains(content, "---") {
			parts := strings.SplitN(content, "---", 3)
			if len(parts) >= 3 {
				body = strings.TrimSpace(parts[2])
			}
		}
		skill := Skill{
			Name:        meta["name"],
			Description: meta["description"],
			Invocation:  meta["invocation"],
			Body:        body,
			Path:        skillPath,
		}
		found = append(found, skill)
	}
	return found
}

// Discover 发现技能
func (sm *SkillsManager) Discover() {
	scanOrder := []string{
		filepath.Join(sm.workspaceDir, "skills"),           // 内置技能
		filepath.Join(sm.workspaceDir, ".skills"),          // 托管技能
		filepath.Join(sm.workspaceDir, ".agents", "skills"),  // 个人 agent 技能
		filepath.Join(".", ".agents", "skills"),      // 项目 agent 技能
		filepath.Join(".", "skills"),                  // 工作区技能
	}
	
	seen := make(map[string]Skill)
	for _, dir := range scanOrder {
		skills := sm._scanDir(dir)
		for _, skill := range skills {
			seen[skill.Name] = skill
		}
	}
	
	// 转换为切片并限制数量
	sm.skills = make([]Skill, 0, len(seen))
	for _, skill := range seen {
		sm.skills = append(sm.skills, skill)
	}
	if len(sm.skills) > MAX_SKILLS {
		sm.skills = sm.skills[:MAX_SKILLS]
	}
}

// FormatPromptBlock 格式化技能为提示词块
func (sm *SkillsManager) FormatPromptBlock() string {
	if len(sm.skills) == 0 {
		return ""
	}
	
	lines := []string{"## Available Skills", ""}
	total := 0
	
	for _, skill := range sm.skills {
		block := "### Skill: " + skill.Name + "\n"
		block += "Description: " + skill.Description + "\n"
		block += "Invocation: " + skill.Invocation + "\n"
		if skill.Body != "" {
			block += "\n" + skill.Body + "\n"
		}
		block += "\n"
		
		if total+len(block) > MAX_SKILLS_PROMPT {
			lines = append(lines, "(... more skills truncated)")
			break
		}
		
		lines = append(lines, block)
		total += len(block)
	}
	
	return strings.Join(lines, "\n")
}

// GetSkills 获取所有技能
func (sm *SkillsManager) GetSkills() []Skill {
	return sm.skills
}
