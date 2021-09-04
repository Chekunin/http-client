package http_client

import (
	"encoding/json"
)

type Err struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Meta    interface{} `json:"meta"`
}

func (e Err) Error() string { return e.Message }

func (e Err) MarshalJSON() ([]byte, error) {
	ret := map[string]interface{}{
		"code":    e.Code,
		"message": e.Message,
	}
	if e.Meta != nil {
		ret["meta"] = e.Meta
	}
	return json.Marshal(ret)
}

func (e Err) String() string {
	data, _ := e.MarshalJSON()
	return string(data)
}

func (e Err) Is(target error) bool {
	if err2, ok := target.(Err); ok {
		return e.Code == err2.Code
	}
	return false
}

func NewErr(code int, message string, meta interface{}) Err {
	return Err{
		Code:    code,
		Message: message,
		Meta:    meta,
	}
}
