// Package mcpserver implements a minimal Model Context Protocol server over
// stdio (JSON-RPC 2.0), exposing asacli commands 1:1 as tools. Tool calls
// re-exec the asacli binary with ASACLI_OUTPUT=json so agents always get
// machine-readable results, and mutations still require explicit
// confirm=true (mapped to --confirm) — the same safety contract humans get.
package mcpserver

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Tool is one exposed command.
type Tool struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Argv        []string `json:"-"` // fixed command path, e.g. ["campaigns","list"]
	Positional  []string `json:"-"` // ordered positional arg names
	Flags       []Flag   `json:"-"`
	Mutating    bool     `json:"-"`
}

// Flag is one accepted flag for a tool.
type Flag struct {
	Name, Type, Description string
}

// Serve runs the stdio loop until EOF.
func Serve(tools []Tool, version string) error {
	in := bufio.NewReaderSize(os.Stdin, 1<<20)
	out := os.Stdout
	for {
		line, err := in.ReadBytes('\n')
		if len(line) == 0 && err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var req struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}
		if req.ID == nil { // notification
			continue
		}
		result, rpcErr := handle(tools, version, req.Method, req.Params)
		resp := map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(req.ID)}
		if rpcErr != nil {
			resp["error"] = rpcErr
		} else {
			resp["result"] = result
		}
		b, _ := json.Marshal(resp)
		fmt.Fprintln(out, string(b))
	}
}

func handle(tools []Tool, version, method string, params json.RawMessage) (any, map[string]any) {
	switch method {
	case "initialize":
		return map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "asacli", "version": version},
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		var defs []map[string]any
		for _, t := range tools {
			props := map[string]any{}
			var required []string
			for _, p := range t.Positional {
				props[p] = map[string]any{"type": "string", "description": "positional argument " + p}
				required = append(required, p)
			}
			for _, f := range t.Flags {
				props[f.Name] = map[string]any{"type": f.Type, "description": f.Description}
			}
			if t.Mutating {
				props["confirm"] = map[string]any{"type": "boolean",
					"description": "must be true to execute; false/omitted runs --dry-run"}
			}
			schema := map[string]any{"type": "object", "properties": props}
			if len(required) > 0 {
				schema["required"] = required
			}
			defs = append(defs, map[string]any{
				"name": t.Name, "description": t.Description, "inputSchema": schema,
			})
		}
		return map[string]any{"tools": defs}, nil
	case "tools/call":
		var call struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(params, &call); err != nil {
			return nil, map[string]any{"code": -32602, "message": "invalid params"}
		}
		for _, t := range tools {
			if t.Name == call.Name {
				text, isErr := run(t, call.Arguments)
				return map[string]any{
					"content": []map[string]any{{"type": "text", "text": text}},
					"isError": isErr,
				}, nil
			}
		}
		return nil, map[string]any{"code": -32602, "message": "unknown tool " + call.Name}
	default:
		return nil, map[string]any{"code": -32601, "message": "method not found: " + method}
	}
}

func run(t Tool, args map[string]any) (string, bool) {
	self, err := os.Executable()
	if err != nil {
		return "cannot locate asacli binary: " + err.Error(), true
	}
	argv := append([]string{}, t.Argv...)
	for _, p := range t.Positional {
		v, ok := args[p]
		if !ok {
			return "missing required argument: " + p, true
		}
		argv = append(argv, fmt.Sprint(v))
	}
	for _, f := range t.Flags {
		v, ok := args[f.Name]
		if !ok {
			continue
		}
		if f.Type == "boolean" {
			if b, ok := v.(bool); ok && b {
				argv = append(argv, "--"+f.Name)
			}
			continue
		}
		argv = append(argv, "--"+f.Name, fmt.Sprint(v))
	}
	if t.Mutating {
		if b, ok := args["confirm"].(bool); ok && b {
			argv = append(argv, "--confirm")
		} else {
			argv = append(argv, "--dry-run")
		}
	}
	cmd := exec.Command(self, argv...)
	cmd.Env = append(os.Environ(), "ASACLI_OUTPUT=json")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	text := strings.TrimSpace(stdout.String())
	if errText := strings.TrimSpace(stderr.String()); errText != "" {
		if text != "" {
			text += "\n"
		}
		text += errText
	}
	if text == "" {
		text = "(no output)"
	}
	return text, err != nil
}
