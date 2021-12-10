package jsonrpc

import (
	"encoding/json"
)

// RawResponse keeps Result and Error as unparsed JSON
// It is meant to be used to deserialize JSONPRC responses from downstream components
// while Response is meant to be used to craft our own responses to clients.
type RawResponse struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      ID               `json:"id"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   *json.RawMessage `json:"error,omitempty"`
}

// MarshalJSON implements json.Marshaler and adds the "jsonrpc":"2.0"
// property.
func (r RawResponse) MarshalJSON() ([]byte, error) {

	if r.Error != nil {
		response := struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      ID              `json:"id"`
			Error   json.RawMessage `json:"error,omitempty"`
		}{
			JSONRPC: "2.0",
			ID:      r.ID,
			Error:   *r.Error,
		}

		return json.Marshal(response)
	} else {
		response := struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      ID              `json:"id"`
			Result  json.RawMessage `json:"result,omitempty"`
		}{
			JSONRPC: "2.0",
			ID:      r.ID,
			Result:  r.Result,
		}

		if response.Result == nil {
			response.Result = jsonNull
		}

		return json.Marshal(response)
	}
}

// UnmarshalJSON implements json.Unmarshaler.
func (r *RawResponse) UnmarshalJSON(data []byte) error {
	type tmpType RawResponse

	if err := json.Unmarshal(data, (*tmpType)(r)); err != nil {
		return err
	}
	return nil
}
