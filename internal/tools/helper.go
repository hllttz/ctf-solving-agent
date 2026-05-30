package tools

import "encoding/json"

func unmarshalArgs(argumentsInJSON string, v interface{}) error {
	return json.Unmarshal([]byte(argumentsInJSON), v)
}
