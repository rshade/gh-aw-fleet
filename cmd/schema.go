package cmd

import (
	"fmt"
	"runtime/debug"

	"github.com/rshade/ax-go/contract"
	"github.com/rshade/ax-go/schema"
	"github.com/spf13/cobra"
)

const (
	schemaCommandName   = "__schema"
	mcpSchemaFieldType  = "type"
	mcpSchemaTypeArray  = "array"
	mcpSchemaTypeString = "string"
)

type mcpPositionalArg struct {
	name        string
	description string
	required    bool
	array       bool
}

func newSchemaCmd(root *cobra.Command) *cobra.Command {
	var as string
	c := &cobra.Command{
		Use:   schemaCommandName,
		Short: "Emit the AX machine-discoverability schema",
		Example: "  " + rootCommandName + " " + schemaCommandName + "\n" +
			"  " + rootCommandName + " " + schemaCommandName + " --as=mcp",
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			switch as {
			case "", "ax":
				return contract.WriteJSON(cmd.OutOrStdout(),
					schema.BuildSchema(root, schema.WithSchemaVersion(toolVersion())))
			case "mcp":
				return contract.WriteJSON(cmd.OutOrStdout(), buildMCPSchema(root))
			default:
				return contract.NewError(
					cmd.Context(),
					"validation_error",
					fmt.Sprintf("unknown schema format %q", as),
					contract.WithErrorExitCode(contract.ExitValidation),
				)
			}
		},
	}
	c.Flags().StringVar(&as, "as", "ax", "schema format: ax or mcp")
	return c
}

func toolVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" || info.Main.Version == "(devel)" {
		return "dev"
	}
	return info.Main.Version
}

func buildMCPSchema(root *cobra.Command) schema.MCPSchema {
	out := schema.BuildMCPSchema(root)
	for i := range out.Tools {
		addMCPPositionalArgs(&out.Tools[i])
	}
	return out
}

func addMCPPositionalArgs(tool *schema.MCPTool) {
	args := mcpPositionalArgs(tool.Name)
	if len(args) == 0 {
		return
	}

	properties, _ := tool.InputSchema["properties"].(map[string]any)
	if properties == nil {
		properties = map[string]any{}
		tool.InputSchema["properties"] = properties
	}
	for index, arg := range args {
		properties[arg.name] = arg.schema(index)
		if arg.required {
			tool.InputSchema["required"] = appendRequired(tool.InputSchema["required"], arg.name)
		}
	}
}

func mcpPositionalArgs(toolName string) []mcpPositionalArg {
	const repoDescription = "Repository slug in owner/name form."
	switch toolName {
	case rootCommandName + " add":
		return []mcpPositionalArg{{
			name:        diagnosticFieldRepo,
			description: repoDescription,
			required:    true,
		}}
	case rootCommandName + " deploy", rootCommandName + " sync":
		return []mcpPositionalArg{{
			name:        diagnosticFieldRepo,
			description: repoDescription,
			required:    true,
		}}
	case rootCommandName + " status":
		return []mcpPositionalArg{{
			name:        diagnosticFieldRepo,
			description: repoDescription + " When omitted, status covers the whole fleet.",
		}}
	case rootCommandName + " upgrade":
		return []mcpPositionalArg{{
			name:        diagnosticFieldRepo,
			description: repoDescription + " Required unless --all is true.",
		}}
	case rootCommandName + " " + commandConsumption:
		return []mcpPositionalArg{{
			name:        "repos",
			description: "Repository slugs in owner/name form. When omitted, consumption covers the whole fleet.",
			array:       true,
		}}
	default:
		return nil
	}
}

func (arg mcpPositionalArg) schema(index int) map[string]any {
	if arg.array {
		return map[string]any{
			mcpSchemaFieldType: mcpSchemaTypeArray,
			"items":            map[string]any{mcpSchemaFieldType: mcpSchemaTypeString},
			"description":      arg.description,
			"x-cli-positional": true,
			"x-cli-position":   index,
			"x-cli-variadic":   true,
		}
	}
	return map[string]any{
		mcpSchemaFieldType: mcpSchemaTypeString,
		"description":      arg.description,
		"x-cli-positional": true,
		"x-cli-position":   index,
	}
}

func appendRequired(existing any, name string) []string {
	required := []string{}
	switch values := existing.(type) {
	case []string:
		required = append(required, values...)
	case []any:
		for _, value := range values {
			if s, ok := value.(string); ok {
				required = append(required, s)
			}
		}
	}
	for _, value := range required {
		if value == name {
			return required
		}
	}
	return append(required, name)
}
