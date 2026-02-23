package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"xbot/llm"
)

// SkillTool 技能管理工具
type SkillTool struct {
	store *SkillStore
}

// NewSkillTool 创建 SkillTool
func NewSkillTool(store *SkillStore) *SkillTool {
	return &SkillTool{store: store}
}

func (t *SkillTool) Name() string { return "Skill" }

func (t *SkillTool) Description() string {
	return `Manage reusable skills. Each skill is a directory (skills/{name}/) containing a SKILL.md file with YAML frontmatter and optional subdirectories (scripts/, references/, assets/).

SKILL.md format:
---
name: skill-name
description: Short description used for triggering/discovery
---
(markdown body with instructions, loaded into system prompt on activation)

Actions:
- list: List all available skills and their activation status
- show: Display the full SKILL.md content of a specific skill
- create: Create or update a skill (provide name and content with YAML frontmatter)
- delete: Delete a skill (removes entire directory)
- activate: Load a skill's body into the current session system prompt
- deactivate: Unload a skill from the current session`
}

func (t *SkillTool) Parameters() []llm.ToolParam {
	return []llm.ToolParam{
		{Name: "action", Type: "string", Description: "Action: list, show, create, delete, activate, deactivate", Required: true},
		{Name: "name", Type: "string", Description: "Skill name (used as directory name, e.g. 'my-skill' creates skills/my-skill/SKILL.md)", Required: false},
		{Name: "content", Type: "string", Description: "SKILL.md content for create action. Should include YAML frontmatter (---\\nname: ...\\ndescription: ...\\n---) followed by markdown body.", Required: false},
	}
}

func (t *SkillTool) Execute(ctx *ToolContext, input string) (*ToolResult, error) {
	var params struct {
		Action  string `json:"action"`
		Name    string `json:"name"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	switch params.Action {
	case "list":
		return t.listSkills()
	case "show":
		return t.showSkill(params.Name)
	case "create":
		return t.createSkill(params.Name, params.Content)
	case "delete":
		return t.deleteSkill(params.Name)
	case "activate":
		return t.activateSkill(params.Name)
	case "deactivate":
		return t.deactivateSkill(params.Name)
	default:
		return nil, fmt.Errorf("unknown action: %q (valid: list, show, create, delete, activate, deactivate)", params.Action)
	}
}

func (t *SkillTool) listSkills() (*ToolResult, error) {
	skills, err := t.store.ListSkills()
	if err != nil {
		return nil, err
	}
	if len(skills) == 0 {
		return NewResult("No skills found. Use action 'create' to create one."), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Available skills (%d):\n", len(skills))
	for _, s := range skills {
		status := "  "
		if s.Active {
			status = "✓ "
		}
		fmt.Fprintf(&sb, "  %s%-16s %s\n", status, s.Name, s.Description)
	}

	active := t.store.ActiveNames()
	if len(active) > 0 {
		fmt.Fprintf(&sb, "\nActive: %s", strings.Join(active, ", "))
	}

	return NewResult(sb.String()), nil
}

func (t *SkillTool) showSkill(name string) (*ToolResult, error) {
	if name == "" {
		return nil, fmt.Errorf("name is required for show action")
	}
	content, err := t.store.GetSkillContent(name)
	if err != nil {
		return nil, err
	}
	return NewResult(fmt.Sprintf("=== Skill: %s ===\nPath: %s\n\n%s", name, t.store.GetSkillDir(name), content)), nil
}

func (t *SkillTool) createSkill(name, content string) (*ToolResult, error) {
	if name == "" {
		return nil, fmt.Errorf("name is required for create action")
	}
	if content == "" {
		return nil, fmt.Errorf("content is required for create action")
	}
	// 如果内容不含 YAML frontmatter，自动添加
	if !strings.HasPrefix(strings.TrimSpace(content), "---") {
		content = fmt.Sprintf("---\nname: %s\ndescription: %s skill\n---\n\n%s", name, name, content)
	}
	if err := t.store.SaveSkill(name, content); err != nil {
		return nil, err
	}
	return NewResult(fmt.Sprintf("Skill %q created/updated at skills/%s/SKILL.md. Use activate action to enable it.", name, name)), nil
}

func (t *SkillTool) deleteSkill(name string) (*ToolResult, error) {
	if name == "" {
		return nil, fmt.Errorf("name is required for delete action")
	}
	if err := t.store.DeleteSkill(name); err != nil {
		return nil, err
	}
	return NewResult(fmt.Sprintf("Skill %q deleted.", name)), nil
}

func (t *SkillTool) activateSkill(name string) (*ToolResult, error) {
	if name == "" {
		return nil, fmt.Errorf("name is required for activate action")
	}
	if err := t.store.Activate(name); err != nil {
		return nil, err
	}
	return NewResult(fmt.Sprintf("Skill %q activated. Its instructions are now part of the system prompt.", name)), nil
}

func (t *SkillTool) deactivateSkill(name string) (*ToolResult, error) {
	if name == "" {
		return nil, fmt.Errorf("name is required for deactivate action")
	}
	t.store.Deactivate(name)
	return NewResult(fmt.Sprintf("Skill %q deactivated.", name)), nil
}
