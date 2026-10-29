package main

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

const (
	skillsGuidePath = "../docs/kagent-skills.md"
	skillsOnlyRoot  = "opt/kagent-tools-skills"
	// kagent go/api/v1alpha3 MCPToolBinding.Tools has MaxItems=50.
	maxToolsPerBinding = 50
)

var yamlCodeBlock = regexp.MustCompile("(?s)```yaml\n(.*?)```")

// Mirrors the subset of kagent go/api/v1alpha3 AgentTemplate the guide uses.
type agentTemplateExample struct {
	Kind string `json:"kind"`
	Spec struct {
		ModelConfig *struct {
			Name string `json:"name"`
		} `json:"modelConfig"`
		Tools []struct {
			MCP *struct {
				Server struct {
					APIGroup string `json:"apiGroup"`
					Kind     string `json:"kind"`
					Name     string `json:"name"`
				} `json:"server"`
				Tools []string `json:"tools"`
			} `json:"mcp"`
		} `json:"tools"`
		Plugins []struct {
			Source struct {
				OCI  string `json:"oci"`
				Path string `json:"path"`
			} `json:"source"`
			Skills []string `json:"skills"`
		} `json:"plugins"`
	} `json:"spec"`
}

func agentTemplateExamples(t *testing.T) []agentTemplateExample {
	t.Helper()
	raw, err := os.ReadFile(skillsGuidePath)
	require.NoError(t, err)
	var templates []agentTemplateExample
	for _, block := range yamlCodeBlock.FindAllStringSubmatch(string(raw), -1) {
		for _, document := range strings.Split(block[1], "\n---\n") {
			var template agentTemplateExample
			require.NoError(t, yaml.Unmarshal([]byte(document), &template))
			if template.Kind == "AgentTemplate" {
				templates = append(templates, template)
			}
		}
	}
	return templates
}

func TestSkillsGuideAgentTemplatesAreValid(t *testing.T) {
	templates := agentTemplateExamples(t)
	require.Len(t, templates, 2, "expected one full-root and one skills-only example")
	knownTools := registeredTools(t)

	for index, template := range templates {
		require.NotNilf(t, template.Spec.ModelConfig, "example %d: managed harnesses require modelConfig", index)
		require.NotEmpty(t, template.Spec.ModelConfig.Name, "example %d", index)
		require.NotEmpty(t, template.Spec.Plugins, "example %d", index)

		boundTools := map[string]bool{}
		for _, binding := range template.Spec.Tools {
			require.NotNil(t, binding.MCP, "example %d", index)
			require.Equal(t, "RemoteMCPServer", binding.MCP.Server.Kind, "example %d", index)
			require.Empty(t, binding.MCP.Server.APIGroup, "example %d: kagent rejects an apiGroup on RemoteMCPServer refs", index)
			require.LessOrEqual(t, len(binding.MCP.Tools), maxToolsPerBinding, "example %d", index)
			for _, toolName := range binding.MCP.Tools {
				require.Containsf(t, knownTools, toolName, "example %d binds unknown tool %q", index, toolName)
				boundTools[toolName] = true
			}
		}

		for _, plugin := range template.Spec.Plugins {
			image := map[string]string{
				skillsOnlyRoot:     "ghcr.io/kagent-dev/kagent/tools-skills@sha256:",
				"opt/kagent-tools": "ghcr.io/kagent-dev/kagent/tools@sha256:",
			}[plugin.Source.Path]
			require.NotEmptyf(t, image, "example %d: unknown plugin root %q", index, plugin.Source.Path)
			require.Truef(t, strings.HasPrefix(plugin.Source.OCI, image), "example %d: %s must come from %s<digest>", index, plugin.Source.Path, image)
			if plugin.Source.Path != skillsOnlyRoot || len(boundTools) == 0 {
				continue
			}
			for _, skillName := range plugin.Skills {
				for _, toolName := range referencedTools(readSkill(t, skillName)) {
					require.Truef(t, boundTools[toolName], "example %d: skill %s uses %s, which the tools list omits", index, skillName, toolName)
				}
			}
		}
	}
}
