//go:build accelerator

package main

import (
	"encoding/json"
	"fmt"

	"kubikles/pkg/agent"
	"kubikles/pkg/server"
)

type AppMethodCaller struct{ app *App }

var _ server.MethodCaller = (*AppMethodCaller)(nil)

func NewAppMethodCaller(app *App) *AppMethodCaller { return &AppMethodCaller{app: app} }
func unmarshalArg[T any](args []json.RawMessage, index int) (T, error) {
	var v T
	if index >= len(args) || args[index] == nil {
		return v, nil
	}
	if err := json.Unmarshal(args[index], &v); err != nil {
		return v, fmt.Errorf("argument %d: %w", index, err)
	}
	return v, nil
}
func (c *AppMethodCaller) CallMethod(call agent.AuthenticatedCallContext, method string, args []json.RawMessage) (interface{}, error) {
	switch method {
	case "ListSecretsMetadata":
		p0, e := unmarshalArg[string](args, 0)
		if e != nil {
			return nil, e
		}
		p1, e := unmarshalArg[string](args, 1)
		if e != nil {
			return nil, e
		}
		return c.app.ListSecretsMetadata(p0, p1)
	case "ListHelmReleaseMetadata":
		p0, e := unmarshalArg[string](args, 0)
		if e != nil {
			return nil, e
		}
		p1, e := unmarshalArg[string](args, 1)
		if e != nil {
			return nil, e
		}
		return c.app.ListAcceleratorHelmReleaseMetadata(p0, p1)
	case "GetSecretData":
		p0, e := unmarshalArg[string](args, 0)
		if e != nil {
			return nil, e
		}
		p1, e := unmarshalArg[string](args, 1)
		if e != nil {
			return nil, e
		}
		return c.app.GetSecretData(p0, p1)
	case "GetSecretYaml":
		p0, e := unmarshalArg[string](args, 0)
		if e != nil {
			return nil, e
		}
		p1, e := unmarshalArg[string](args, 1)
		if e != nil {
			return nil, e
		}
		return c.app.GetSecretYaml(p0, p1)
	case "CancelListRequest":
		p0, e := unmarshalArg[string](args, 0)
		if e != nil {
			return nil, e
		}
		return c.app.CancelListRequest(p0), nil
	case "SubscribeSecretWatcher":
		p0, e := unmarshalArg[string](args, 0)
		if e != nil {
			return nil, e
		}
		p1, e := unmarshalArg[bool](args, 1)
		if e != nil {
			return nil, e
		}
		return c.app.SubscribeSecretWatcher(call, p0, p1)
	case "UnsubscribeSecretWatcher":
		p0, e := unmarshalArg[string](args, 0)
		if e != nil {
			return nil, e
		}
		return nil, c.app.UnsubscribeSecretWatcher(call, p0)
	default:
		return nil, fmt.Errorf("method %q is not available in Accelerator", method)
	}
}
