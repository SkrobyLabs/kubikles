package acceleratoracceptance

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

const maximumAcceptanceJSONBytes = 1 << 20

func readStrictJSON(reader io.Reader) ([]byte, error) {
	if reader == nil {
		return nil, errors.New("invalid JSON")
	}
	data, err := io.ReadAll(io.LimitReader(reader, maximumAcceptanceJSONBytes+1))
	if err != nil || len(data) == 0 || len(data) > maximumAcceptanceJSONBytes {
		return nil, errors.New("invalid JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := consumeUniqueJSONValue(decoder); err != nil {
		return nil, errors.New("invalid JSON")
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("invalid JSON")
	}
	return data, nil
}

func consumeUniqueJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, composite := token.(json.Delim)
	if !composite {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, keyErr := decoder.Token()
			key, ok := keyToken.(string)
			if keyErr != nil || !ok {
				return errors.New("invalid object key")
			}
			if _, duplicate := seen[key]; duplicate {
				return errors.New("duplicate object key")
			}
			seen[key] = struct{}{}
			if err := consumeUniqueJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') {
			return errors.New("invalid object")
		}
	case '[':
		for decoder.More() {
			if err := consumeUniqueJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			return errors.New("invalid array")
		}
	default:
		return errors.New("invalid delimiter")
	}
	return nil
}
