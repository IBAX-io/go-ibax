package jsonrpc

import (
	"encoding/json"

	"github.com/pkg/errors"
)

type Request struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	ID      ID     `json:"id"`
	Params  Params `json:"params"`
}

type BatchRequest []*Request

type RequestWithNetwork struct {
	*Request
	Network string `json:"network"`
}

func NewRequest() *Request {
	return &Request{JSONRPC: "2.0", ID: ID{Num: 1}}
}

// MakeRequest builds a Request from all its parts, but returns an error if the
// params cannot be marshalled.
func MakeRequest(id int, method string, params ...any) (*Request, error) {
	p, err := MakeParams(params...)
	if err != nil {
		return nil, err
	}

	return &Request{
		JSONRPC: "2.0",
		ID:      ID{Num: uint64(id)},
		Method:  method,
		Params:  p,
	}, nil
}

// MustRequest builds a request from all its parts but panics if the params cannot be marshaled,
// so should only be used with well-known parameter data.
func MustRequest(id int, method string, params ...any) *Request {
	r, err := MakeRequest(id, method, params...)
	if err != nil {
		panic(err)
	}

	return r
}

// MarshalJSON implements json.Marshaler and adds the "jsonrpc":"2.0"
// property.
func (r Request) MarshalJSON() ([]byte, error) {
	r2 := struct {
		Method  string `json:"method"`
		Params  Params `json:"params,omitempty"`
		ID      *ID    `json:"id,omitempty"`
		JSONRPC string `json:"jsonrpc"`
	}{
		Method:  r.Method,
		Params:  r.Params,
		JSONRPC: "2.0",
	}
	r2.ID = &r.ID
	return json.Marshal(r2)
}

// UnmarshalJSON implements json.Unmarshaler.
func (r *Request) UnmarshalJSON(data []byte) error {
	var r2 struct {
		Method string           `json:"method"`
		Params *json.RawMessage `json:"params,omitempty"`
		Meta   *json.RawMessage `json:"meta,omitempty"`
		ID     *ID              `json:"id"`
	}

	// Detect if the "params" field is JSON "null" or just not present
	// by seeing if the field gets overwritten to nil.
	r2.Params = &json.RawMessage{}

	if err := json.Unmarshal(data, &r2); err != nil {
		return err
	}
	r.Method = r2.Method
	if r2.Params == nil {
		r.Params = nil
	} else if len(*r2.Params) == 0 {
		r.Params = nil
	} else {
		err := json.Unmarshal(*r2.Params, &r.Params)
		if err != nil {
			return err
		}
	}

	if r2.Method == "" {
		return errors.New("request is missing method")
	}

	if r2.ID == nil {
		return errors.New("request is missing ID")
	} else {
		r.ID = *r2.ID
	}

	r.JSONRPC = "2.0"
	return nil
}

// MarshalJSON implements json.Marshaler and adds the "jsonrpc":"2.0"
// property.
func (r RequestWithNetwork) MarshalJSON() ([]byte, error) {
	r2 := struct {
		Method  string `json:"method"`
		Params  Params `json:"params,omitempty"`
		ID      *ID    `json:"id,omitempty"`
		JSONRPC string `json:"jsonrpc"`
		Network string `json:"network"`
	}{
		Method:  r.Method,
		Params:  r.Params,
		Network: r.Network,
		JSONRPC: "2.0",
	}
	r2.ID = &r.ID
	return json.Marshal(r2)
}

// UnmarshalJSON implements json.Unmarshaler.
func (r *RequestWithNetwork) UnmarshalJSON(data []byte) error {
	var r2 struct {
		Method  string           `json:"method"`
		Params  *json.RawMessage `json:"params,omitempty"`
		Meta    *json.RawMessage `json:"meta,omitempty"`
		Network string           `json:"network"`
		ID      *ID              `json:"id"`
	}

	// Detect if the "params" field is JSON "null" or just not present
	// by seeing if the field gets overwritten to nil.
	r2.Params = &json.RawMessage{}
	r2.Network = r.Network

	if err := json.Unmarshal(data, &r2); err != nil {
		return err
	}
	r.Method = r2.Method
	if r2.Params == nil {
		r.Params = nil
	} else if len(*r2.Params) == 0 {
		r.Params = nil
	} else {
		err := json.Unmarshal(*r2.Params, &r.Params)
		if err != nil {
			return err
		}
	}

	if r2.ID == nil {
		return errors.New("request is missing ID")
	} else {
		r.ID = *r2.ID
	}
	return nil
}
