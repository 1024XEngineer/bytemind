package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	extensionspkg "bytemind/internal/extensions"
	toolspkg "bytemind/internal/tools"
)

func newExtensionError(code extensionspkg.ErrorCode, message string, err error) error {
	return &extensionspkg.ExtensionError{
		Code:    code,
		Message: strings.TrimSpace(message),
		Err:     err,
	}
}

func toExtensionError(err error, fallbackCode extensionspkg.ErrorCode, fallbackMessage string) error {
	if err == nil {
		return nil
	}
	var extErr *extensionspkg.ExtensionError
	if errors.As(err, &extErr) {
		return err
	}
	message := strings.TrimSpace(err.Error())
	if message == "" {
		message = strings.TrimSpace(fallbackMessage)
	}
	return newExtensionError(fallbackCode, message, err)
}

func mapClientErrorToExtensionCode(err error) extensionspkg.ErrorCode {
	var clientErr *ClientError
	if !errors.As(err, &clientErr) || clientErr == nil {
		return extensionspkg.ErrCodeLoadFailed
	}
	switch clientErr.Code {
	case ClientErrorInvalidConfig, ClientErrorInvalidArgs:
		return extensionspkg.ErrCodeInvalidManifest
	case ClientErrorPermission:
		return extensionspkg.ErrCodeConflict
	case ClientErrorHandshakeFailed, ClientErrorListToolsFailed, ClientErrorCallFailed:
		return extensionspkg.ErrCodeLoadFailed
	case ClientErrorTimeout:
		return extensionspkg.ErrCodeBusy
	default:
		return extensionspkg.ErrCodeLoadFailed
	}
}

func mapClientErrorToToolExecError(err error) error {
	if err == nil {
		return nil
	}
	var clientErr *ClientError
	if !errors.As(err, &clientErr) || clientErr == nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return toolspkg.NewToolExecError(toolspkg.ToolErrorTimeout, "mcp tool request timed out", true, err)
		}
		return toolspkg.NewToolExecError(toolspkg.ToolErrorToolFailed, err.Error(), true, err)
	}
	switch clientErr.Code {
	case ClientErrorTimeout:
		return toolspkg.NewToolExecError(toolspkg.ToolErrorTimeout, clientErr.Error(), true, clientErr)
	case ClientErrorPermission:
		return toolspkg.NewToolExecError(toolspkg.ToolErrorPermissionDenied, clientErr.Error(), false, clientErr)
	case ClientErrorInvalidArgs:
		return toolspkg.NewToolExecError(toolspkg.ToolErrorInvalidArgs, clientErr.Error(), false, clientErr)
	default:
		return toolspkg.NewToolExecError(toolspkg.ToolErrorToolFailed, clientErr.Error(), true, clientErr)
	}
}

func contextError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return nil
}

func filterValidToolDescriptors(tools []ToolDescriptor) ([]ToolDescriptor, int) {
	if len(tools) == 0 {
		return nil, 0
	}
	valid := make([]ToolDescriptor, 0, len(tools))
	skipped := 0
	for _, descriptor := range tools {
		name := strings.TrimSpace(descriptor.Name)
		if name == "" {
			skipped++
			continue
		}
		item := ToolDescriptor{
			Name:        name,
			Description: strings.TrimSpace(descriptor.Description),
			InputSchema: normalizedSchema(descriptor.InputSchema),
		}
		valid = append(valid, item)
	}
	return valid, skipped
}

func cloneToolDescriptors(items []ToolDescriptor) []ToolDescriptor {
	if len(items) == 0 {
		return nil
	}
	out := make([]ToolDescriptor, 0, len(items))
	for _, item := range items {
		out = append(out, ToolDescriptor{
			Name:        item.Name,
			Description: item.Description,
			InputSchema: cloneMap(item.InputSchema),
		})
	}
	return out
}

func normalizedSchema(schema map[string]any) map[string]any {
	if len(schema) == 0 {
		return map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		}
	}
	cloned := cloneMap(schema)
	if _, ok := cloned["type"]; !ok {
		cloned["type"] = "object"
	}
	return cloned
}

func marshalJSONInline(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(data)
}

func formatHealthMessage(prefix string, payload any) string {
	prefix = strings.TrimSpace(prefix)
	if payload == nil {
		return prefix
	}
	if encoded := strings.TrimSpace(marshalJSONInline(payload)); encoded != "" {
		if prefix == "" {
			return encoded
		}
		return fmt.Sprintf("%s: %s", prefix, encoded)
	}
	return prefix
}
