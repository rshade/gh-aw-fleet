package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	axschema "github.com/rshade/ax-go/schema"
)

type schemaExitCoder interface {
	ExitCode() int
}

func TestSchemaCommandEmitsAXSchema(t *testing.T) {
	stdout, stderr, err := executeSchemaCommand("__schema")
	if err != nil {
		t.Fatalf("__schema returned error: %v\nstderr: %s", err, stderr)
	}

	var got axschema.Schema
	if decodeErr := json.Unmarshal([]byte(stdout), &got); decodeErr != nil {
		t.Fatalf("__schema stdout is not valid schema JSON: %v\nstdout: %s", decodeErr, stdout)
	}
	if got.Tool != rootCommandName {
		t.Fatalf("tool = %q; want %s", got.Tool, rootCommandName)
	}
	if got.Version == "" {
		t.Fatal("version is empty")
	}

	wantCommands := []string{"list", "status", "add", templateCommandName, "deploy", "sync", "upgrade", "consumption"}
	commandNames := map[string]bool{}
	for _, cmd := range got.Command.Commands {
		name := strings.Fields(cmd.Use)[0]
		commandNames[name] = true
	}
	for _, want := range wantCommands {
		if !commandNames[want] {
			t.Errorf("__schema command tree missing %q; commands=%v", want, commandNames)
		}
	}

	wantFlags := []string{"dir", "log-level", "log-format", "output"}
	flagNames := map[string]bool{}
	for _, flag := range got.Command.Flags {
		flagNames[flag.Name] = true
	}
	for _, want := range wantFlags {
		if !flagNames[want] {
			t.Errorf("__schema root flags missing %q; flags=%v", want, flagNames)
		}
	}
}

func TestSchemaCommandEmitsMCPSchema(t *testing.T) {
	stdout, stderr, err := executeSchemaCommand("__schema", "--as", "mcp")
	if err != nil {
		t.Fatalf("__schema --as mcp returned error: %v\nstderr: %s", err, stderr)
	}

	var got axschema.MCPSchema
	if decodeErr := json.Unmarshal([]byte(stdout), &got); decodeErr != nil {
		t.Fatalf("__schema --as mcp stdout is not valid MCP JSON: %v\nstdout: %s", decodeErr, stdout)
	}
	if len(got.Tools) == 0 {
		t.Fatal("MCP tools list is empty")
	}
	if !slices.ContainsFunc(got.Tools, func(tool axschema.MCPTool) bool {
		return tool.Name == rootCommandName
	}) {
		t.Fatalf("MCP tools list missing root tool; tools=%v", got.Tools)
	}
}

func TestSchemaCommandMCPIncludesPositionalArgs(t *testing.T) {
	stdout, stderr, err := executeSchemaCommand("__schema", "--as", "mcp")
	if err != nil {
		t.Fatalf("__schema --as mcp returned error: %v\nstderr: %s", err, stderr)
	}

	var got axschema.MCPSchema
	if decodeErr := json.Unmarshal([]byte(stdout), &got); decodeErr != nil {
		t.Fatalf("__schema --as mcp stdout is not valid MCP JSON: %v\nstdout: %s", decodeErr, stdout)
	}

	for _, name := range []string{
		rootCommandName + " add",
		rootCommandName + " deploy",
		rootCommandName + " sync",
	} {
		tool := requireMCPTool(t, got, name)
		property := requireMCPProperty(t, tool, "repo")
		assertMCPStringProperty(t, name, property)
		if !mcpRequired(tool, "repo") {
			t.Fatalf("%s inputSchema.required missing repo; schema=%v", name, tool.InputSchema)
		}
	}

	for _, name := range []string{
		rootCommandName + " status",
		rootCommandName + " upgrade",
	} {
		tool := requireMCPTool(t, got, name)
		property := requireMCPProperty(t, tool, "repo")
		assertMCPStringProperty(t, name, property)
		if mcpRequired(tool, "repo") {
			t.Fatalf("%s inputSchema.required unexpectedly includes repo; schema=%v", name, tool.InputSchema)
		}
	}

	consumption := requireMCPTool(t, got, rootCommandName+" "+commandConsumption)
	repos := requireMCPProperty(t, consumption, "repos")
	if gotType, _ := repos["type"].(string); gotType != "array" {
		t.Fatalf("consumption repos type = %q; want array", gotType)
	}
	if variadic, _ := repos["x-cli-variadic"].(bool); !variadic {
		t.Fatalf("consumption repos missing x-cli-variadic marker; property=%v", repos)
	}
	if mcpRequired(consumption, "repos") {
		t.Fatalf("consumption inputSchema.required unexpectedly includes repos; schema=%v", consumption.InputSchema)
	}
}

func TestSchemaCommandIncludesStrictFlags(t *testing.T) {
	stdout, stderr, err := executeSchemaCommand("__schema")
	if err != nil {
		t.Fatalf("__schema returned error: %v\nstderr: %s", err, stderr)
	}

	var got axschema.Schema
	if decodeErr := json.Unmarshal([]byte(stdout), &got); decodeErr != nil {
		t.Fatalf("__schema stdout is not valid schema JSON: %v\nstdout: %s", decodeErr, stdout)
	}

	for _, use := range []string{"deploy <repo>", "sync <repo>", "upgrade [repo|--all]"} {
		cmd := requireSchemaCommand(t, got.Command, use)
		flag := requireSchemaFlag(t, cmd, "strict")
		if flag.Type != "bool" {
			t.Fatalf("%s --strict type = %q; want bool", use, flag.Type)
		}
		if flag.Default != "false" {
			t.Fatalf("%s --strict default = %q; want false", use, flag.Default)
		}
		for _, want := range []string{"HIGH Layer 1 security findings", "does not change gh aw compile --strict"} {
			if !strings.Contains(flag.Usage, want) {
				t.Fatalf("%s --strict usage = %q; want substring %q", use, flag.Usage, want)
			}
		}
	}
}

func TestSchemaCommandRejectsUnknownFormat(t *testing.T) {
	_, _, err := executeSchemaCommand("__schema", "--as", "bogus")
	if err == nil {
		t.Fatal("__schema --as bogus succeeded; want error")
	}
	var exitCoder schemaExitCoder
	if !errors.As(err, &exitCoder) {
		t.Fatalf("error %T does not expose an exit code", err)
	}
	if exitCoder.ExitCode() == 0 {
		t.Fatalf("exit code = 0; want non-zero")
	}
	if !strings.Contains(err.Error(), `unknown schema format "bogus"`) {
		t.Fatalf("error = %q; want unknown format message", err.Error())
	}
}

func TestSchemaCommandHiddenFromHelp(t *testing.T) {
	root := NewRootCmd()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)

	cmd, _, err := root.Find([]string{"__schema"})
	if err != nil {
		t.Fatalf("find __schema: %v", err)
	}
	if cmd == nil {
		t.Fatal("__schema command not found")
	}
	if !cmd.Hidden {
		t.Fatal("__schema is not hidden")
	}
}

func executeSchemaCommand(args ...string) (string, string, error) {
	root := NewRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)
	err := root.Execute()
	return stdout.String(), stderr.String(), err
}

func requireMCPTool(t *testing.T, schema axschema.MCPSchema, name string) axschema.MCPTool {
	t.Helper()
	for _, tool := range schema.Tools {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("MCP tools list missing %q; tools=%v", name, schema.Tools)
	return axschema.MCPTool{}
}

func requireMCPProperty(t *testing.T, tool axschema.MCPTool, name string) map[string]any {
	t.Helper()
	properties, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("%s inputSchema.properties has type %T; schema=%v",
			tool.Name, tool.InputSchema["properties"], tool.InputSchema)
	}
	property, ok := properties[name].(map[string]any)
	if !ok {
		t.Fatalf("%s inputSchema.properties missing %q; properties=%v", tool.Name, name, properties)
	}
	return property
}

func requireSchemaCommand(t *testing.T, root axschema.CommandSchema, use string) axschema.CommandSchema {
	t.Helper()
	for _, cmd := range root.Commands {
		if cmd.Use == use {
			return cmd
		}
	}
	t.Fatalf("__schema command tree missing %q", use)
	return axschema.CommandSchema{}
}

func requireSchemaFlag(t *testing.T, cmd axschema.CommandSchema, name string) axschema.FlagSchema {
	t.Helper()
	for _, flag := range cmd.Flags {
		if flag.Name == name {
			return flag
		}
	}
	t.Fatalf("%s missing flag %q", cmd.Use, name)
	return axschema.FlagSchema{}
}

func assertMCPStringProperty(t *testing.T, toolName string, property map[string]any) {
	t.Helper()
	if gotType, _ := property["type"].(string); gotType != "string" {
		t.Fatalf("%s repo type = %q; want string", toolName, gotType)
	}
	if positional, _ := property["x-cli-positional"].(bool); !positional {
		t.Fatalf("%s repo missing x-cli-positional marker; property=%v", toolName, property)
	}
}

func mcpRequired(tool axschema.MCPTool, name string) bool {
	values, ok := tool.InputSchema["required"].([]any)
	if !ok {
		return false
	}
	return slices.ContainsFunc(values, func(value any) bool {
		s, ok := value.(string)
		return ok && s == name
	})
}
