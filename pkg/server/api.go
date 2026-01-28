package server

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// MethodCaller is the interface that the App must implement to handle API calls.
// This abstracts the method dispatch mechanism from the server.
type MethodCaller interface {
	// CallMethod invokes a method by name with JSON-encoded arguments.
	// Returns the result (JSON-serializable) and any error.
	CallMethod(methodName string, args []json.RawMessage) (interface{}, error)
}

// ReflectMethodCaller provides a reflection-based implementation of MethodCaller.
// This wraps any struct and exposes its public methods via reflection.
type ReflectMethodCaller struct {
	target      interface{}
	targetValue reflect.Value
}

// NewReflectMethodCaller creates a new reflection-based method caller.
func NewReflectMethodCaller(target interface{}) *ReflectMethodCaller {
	return &ReflectMethodCaller{
		target:      target,
		targetValue: reflect.ValueOf(target),
	}
}

// CallMethod invokes a method on the target using reflection.
func (r *ReflectMethodCaller) CallMethod(methodName string, args []json.RawMessage) (interface{}, error) {
	method := r.targetValue.MethodByName(methodName)
	if !method.IsValid() {
		return nil, fmt.Errorf("method %q not found", methodName)
	}

	methodType := method.Type()
	numIn := methodType.NumIn()

	// Build argument values
	in := make([]reflect.Value, numIn)
	for i := 0; i < numIn; i++ {
		argType := methodType.In(i)
		if i < len(args) && args[i] != nil {
			argValue, err := convertJSONArg(args[i], argType)
			if err != nil {
				return nil, fmt.Errorf("argument %d (%s): %w", i, argType.String(), err)
			}
			in[i] = argValue
		} else {
			// Use zero value for missing arguments
			in[i] = reflect.Zero(argType)
		}
	}

	// Call the method
	results := method.Call(in)

	// Process results
	return processResults(results)
}

// convertJSONArg converts a JSON-encoded argument to the target type.
func convertJSONArg(raw json.RawMessage, targetType reflect.Type) (reflect.Value, error) {
	// Create a new value of the target type
	newValue := reflect.New(targetType)

	// Unmarshal JSON directly into the target type
	if err := json.Unmarshal(raw, newValue.Interface()); err != nil {
		return reflect.Value{}, fmt.Errorf("failed to unmarshal to %s: %w", targetType.String(), err)
	}

	return newValue.Elem(), nil
}

// processResults extracts the return value and error from method results.
func processResults(results []reflect.Value) (interface{}, error) {
	if len(results) == 0 {
		return nil, nil
	}

	// Check if last result is an error
	lastResult := results[len(results)-1]
	if lastResult.Type().Implements(reflect.TypeOf((*error)(nil)).Elem()) {
		if !lastResult.IsNil() {
			return nil, lastResult.Interface().(error)
		}
		// Remove error from results
		if len(results) == 1 {
			return nil, nil
		}
		results = results[:len(results)-1]
	}

	// Return single result or array of results
	if len(results) == 1 {
		return results[0].Interface(), nil
	}

	returnValues := make([]interface{}, len(results))
	for i, r := range results {
		returnValues[i] = r.Interface()
	}
	return returnValues, nil
}

// HasMethod checks if the target has a method with the given name.
func (r *ReflectMethodCaller) HasMethod(methodName string) bool {
	return r.targetValue.MethodByName(methodName).IsValid()
}

// ListMethods returns a list of all public method names on the target.
func (r *ReflectMethodCaller) ListMethods() []string {
	t := r.targetValue.Type()
	methods := make([]string, 0, t.NumMethod())
	for i := 0; i < t.NumMethod(); i++ {
		methods = append(methods, t.Method(i).Name)
	}
	return methods
}
