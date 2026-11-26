package api

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"strconv"
	"strings"

	"github.com/devforth/exceptic/internal/storage/store"
)

//go:embed instructions.md skills/*/SKILL.md
var skillFiles embed.FS

type agentSkill struct {
	name        string
	description string
	content     string
}

// skillPlaceholders are the values of the code that the instructions and
// the skills name, so they never repeat a number.
var skillPlaceholders = strings.NewReplacer(
	"{{default_limit}}", strconv.Itoa(DefaultLimit),
	"{{max_limit}}", strconv.Itoa(MaxLimit),
	"{{max_issue_batch}}", strconv.Itoa(store.MaxIssueBatch),
)

// skills are guides an agent loads on demand (fetch_skill), one topic each,
// named and described by their front matter.
var skills = loadSkills()

func loadSkills() []agentSkill {
	paths, err := fs.Glob(skillFiles, "skills/*/SKILL.md")

	// Embedded at build time: a failure is a programming error.
	if err != nil || len(paths) == 0 {
		panic(fmt.Sprintf("skills: %v", err))
	}

	loaded := make([]agentSkill, 0, len(paths))

	for _, path := range paths {
		content, err := skillFiles.ReadFile(path)

		if err != nil {
			panic(err)
		}

		skill := agentSkill{content: skillPlaceholders.Replace(string(content))}
		frontMatter, _, _ := strings.Cut(strings.TrimPrefix(skill.content, "---\n"), "\n---\n")

		for line := range strings.SplitSeq(frontMatter, "\n") {
			key, value, _ := strings.Cut(line, ":")

			switch key {
			case "name":
				skill.name = strings.TrimSpace(value)
			case "description":
				skill.description = strings.TrimSpace(value)
			}
		}

		if skill.name == "" || skill.description == "" {
			panic(path + ": front matter needs a name and a description")
		}

		loaded = append(loaded, skill)
	}

	return loaded
}

func skillNames() []string {
	names := make([]string, 0, len(skills))

	for _, skill := range skills {
		names = append(names, skill.name)
	}

	return names
}

// fetchSkill is a tool for agents only, not part of the REST API.
var fetchSkill = Define(Endpoint{
	Name: "fetch_skill",
	Description: "Detailed guidance by name: " + strings.Join(skillNames(), ", ") +
		". Only when the server instructions do not settle the task; reuse a skill already loaded.",
	ReadOnly:   true,
	Transports: TransportMCP,
	Choices:    map[string][]string{"name": skillNames()},
}, readSkill)

// serverInstructions is instructions.md with its placeholders filled in,
// then the list of skills.
func serverInstructions(appName string) string {
	instructions, err := skillFiles.ReadFile("instructions.md")

	if err != nil {
		panic(err)
	}

	var text strings.Builder

	text.WriteString(skillPlaceholders.Replace(strings.ReplaceAll(string(instructions), "{{app}}", appName)))

	for _, skill := range skills {
		fmt.Fprintf(&text, "  - %s: %s\n", skill.name, skill.description)
	}

	return text.String()
}

type fetchSkillInput struct {
	Name string `json:"name" jsonschema:"The skill's name."`
}

func readSkill(_ context.Context, _ Call, input fetchSkillInput) (Text, error) {
	for _, skill := range skills {
		if skill.name == input.Name {
			return Text(skill.content), nil
		}
	}

	return "", ErrNotFound // the schema admits only the skills' names
}
