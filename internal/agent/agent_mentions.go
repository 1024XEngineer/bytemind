package agent

import (
	"fmt"
	"regexp"
	"strings"
)

var agentMentionPattern = regexp.MustCompile(`@(\w[\w-]*)`)

type AgentMention struct {
	Name string
}

func extractAgentMentions(input string, knownAgents map[string]struct{}) []AgentMention {
	matches := agentMentionPattern.FindAllStringSubmatch(input, -1)
	var mentions []AgentMention
	seen := make(map[string]struct{})
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		name := match[1]
		if _, ok := knownAgents[name]; ok {
			if _, already := seen[name]; !already {
				mentions = append(mentions, AgentMention{Name: name})
				seen[name] = struct{}{}
			}
		}
	}
	return mentions
}

func buildAgentMentionReminder(mentions []AgentMention, agentDescs map[string]string) string {
	if len(mentions) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("The user has expressed a desire to invoke the following agent(s):\n")
	for _, m := range mentions {
		desc := agentDescs[m.Name]
		if desc != "" {
			fmt.Fprintf(&b, "- %s: %s\n", m.Name, desc)
		} else {
			fmt.Fprintf(&b, "- %s\n", m.Name)
		}
	}
	b.WriteString("Use the delegate_subagent tool if appropriate, passing in the required context to it.")
	return b.String()
}

func enhanceUserMessageWithAgentMentions(original string, knownAgents map[string]struct{}, agentDescs map[string]string) string {
	mentions := extractAgentMentions(original, knownAgents)
	if len(mentions) == 0 {
		return original
	}
	reminder := buildAgentMentionReminder(mentions, agentDescs)
	return original + "\n<system-reminder>\n" + reminder + "\n</system-reminder>"
}
