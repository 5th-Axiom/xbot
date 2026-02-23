package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	log "xbot/logger"
)

// SkillStore 管理 skill 的存储和激活状态
// Skill 格式: skills/{skill-name}/SKILL.md (含 YAML frontmatter)
// 可选子目录: scripts/, references/, assets/
type SkillStore struct {
	mu     sync.RWMutex
	dir    string            // skills 根目录（DataDir/skills）
	active map[string]string // 已激活的 skill: name -> body content (不含 frontmatter)
}

// NewSkillStore 创建 SkillStore
func NewSkillStore(dir string) *SkillStore {
	os.MkdirAll(dir, 0755)
	return &SkillStore{
		dir:    dir,
		active: make(map[string]string),
	}
}

// Dir 返回 skills 根目录
func (s *SkillStore) Dir() string {
	return s.dir
}

// SkillInfo skill 基本信息
type SkillInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Active      bool   `json:"active"`
	Path        string `json:"path"` // skill 目录路径
}

// ListSkills 列出所有可用的 skill
func (s *SkillStore) ListSkills() ([]SkillInfo, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var skills []SkillInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillDir := filepath.Join(s.dir, e.Name())
		skillFile := filepath.Join(skillDir, "SKILL.md")
		if _, err := os.Stat(skillFile); err != nil {
			continue
		}

		name, desc := s.parseFrontmatter(skillFile)
		if name == "" {
			name = e.Name()
		}
		_, activated := s.active[name]
		skills = append(skills, SkillInfo{
			Name:        name,
			Description: desc,
			Active:      activated,
			Path:        skillDir,
		})
	}
	return skills, nil
}

// parseFrontmatter 从 SKILL.md 解析 YAML frontmatter 中的 name 和 description
// frontmatter 格式:
//
//	---
//	name: skill-name
//	description: ...
//	---
func (s *SkillStore) parseFrontmatter(path string) (name, description string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", ""
	}
	content := string(data)

	// 检查是否以 --- 开头
	if !strings.HasPrefix(strings.TrimSpace(content), "---") {
		return "", ""
	}

	// 找到第二个 ---
	trimmed := strings.TrimSpace(content)
	rest := trimmed[3:] // 跳过第一个 ---
	endIdx := strings.Index(rest, "\n---")
	if endIdx < 0 {
		return "", ""
	}

	frontmatter := rest[:endIdx]
	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "name:") {
			name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
		} else if strings.HasPrefix(line, "description:") {
			description = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
		}
	}
	return name, description
}

// getSkillBody 读取 SKILL.md 的 body 部分（去掉 YAML frontmatter）
func (s *SkillStore) getSkillBody(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	content := string(data)

	// 去掉 frontmatter
	trimmed := strings.TrimSpace(content)
	if strings.HasPrefix(trimmed, "---") {
		rest := trimmed[3:]
		endIdx := strings.Index(rest, "\n---")
		if endIdx >= 0 {
			body := strings.TrimSpace(rest[endIdx+4:]) // 跳过 \n---
			return body, nil
		}
	}
	// 没有 frontmatter，返回全部内容
	return content, nil
}

// GetSkillContent 读取 skill 的完整 SKILL.md 内容
func (s *SkillStore) GetSkillContent(name string) (string, error) {
	skillFile := s.findSkillFile(name)
	if skillFile == "" {
		return "", fmt.Errorf("skill %q not found", name)
	}
	data, err := os.ReadFile(skillFile)
	if err != nil {
		return "", fmt.Errorf("read skill: %w", err)
	}
	return string(data), nil
}

// GetSkillDir 返回 skill 的目录路径
func (s *SkillStore) GetSkillDir(name string) string {
	// 先按 frontmatter name 查找
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillFile := filepath.Join(s.dir, e.Name(), "SKILL.md")
		n, _ := s.parseFrontmatter(skillFile)
		if n == name || e.Name() == name {
			return filepath.Join(s.dir, e.Name())
		}
	}
	return ""
}

// findSkillFile 查找 skill 的 SKILL.md 路径（支持按 name 或目录名匹配）
func (s *SkillStore) findSkillFile(name string) string {
	// 先尝试按目录名直接查找
	direct := filepath.Join(s.dir, name, "SKILL.md")
	if _, err := os.Stat(direct); err == nil {
		return direct
	}

	// 再遍历查找 frontmatter name 匹配的
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillFile := filepath.Join(s.dir, e.Name(), "SKILL.md")
		n, _ := s.parseFrontmatter(skillFile)
		if n == name {
			return skillFile
		}
	}
	return ""
}

// SaveSkill 创建或更新 skill
// 如果 skill 目录不存在，创建 {dir}/{name}/SKILL.md
func (s *SkillStore) SaveSkill(name, content string) error {
	skillDir := filepath.Join(s.dir, name)
	os.MkdirAll(skillDir, 0755)

	skillFile := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("save skill: %w", err)
	}

	// 如果该 skill 已激活，更新内存中的 body
	body, _ := s.getSkillBody(skillFile)
	s.mu.Lock()
	if _, ok := s.active[name]; ok {
		s.active[name] = body
	}
	s.mu.Unlock()

	log.WithField("skill", name).Info("Skill saved")
	return nil
}

// DeleteSkill 删除 skill（整个目录）
func (s *SkillStore) DeleteSkill(name string) error {
	skillDir := s.GetSkillDir(name)
	if skillDir == "" {
		return fmt.Errorf("skill %q not found", name)
	}

	s.Deactivate(name)

	if err := os.RemoveAll(skillDir); err != nil {
		return fmt.Errorf("delete skill: %w", err)
	}
	log.WithField("skill", name).Info("Skill deleted")
	return nil
}

// Activate 激活 skill（加载 SKILL.md body 到内存）
func (s *SkillStore) Activate(name string) error {
	skillFile := s.findSkillFile(name)
	if skillFile == "" {
		return fmt.Errorf("skill %q not found", name)
	}

	body, err := s.getSkillBody(skillFile)
	if err != nil {
		return fmt.Errorf("read skill body: %w", err)
	}

	s.mu.Lock()
	s.active[name] = body
	s.mu.Unlock()

	log.WithField("skill", name).Info("Skill activated")
	return nil
}

// Deactivate 停用 skill
func (s *SkillStore) Deactivate(name string) {
	s.mu.Lock()
	_, existed := s.active[name]
	delete(s.active, name)
	s.mu.Unlock()

	if existed {
		log.WithField("skill", name).Info("Skill deactivated")
	}
}

// ActiveNames 返回当前激活的 skill 名称列表
func (s *SkillStore) ActiveNames() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	names := make([]string, 0, len(s.active))
	for name := range s.active {
		names = append(names, name)
	}
	return names
}

// GetActiveSkillsPrompt 返回所有已激活 skill 的合并 prompt，用于注入系统提示
func (s *SkillStore) GetActiveSkillsPrompt() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.active) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("# Active Skills\n\n")
	for name, body := range s.active {
		fmt.Fprintf(&sb, "## Skill: %s\n\n%s\n\n", name, body)
	}
	return sb.String()
}
