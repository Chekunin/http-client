package http_client

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
)

func GobEncoder(payload interface{}) (io.Reader, error) {
	if payload == nil {
		return nil, nil
	}
	var network bytes.Buffer
	enc := gob.NewEncoder(&network)
	err := enc.Encode(payload)
	if err != nil {
		return nil, fmt.Errorf("gob enc.Encode: %s", err)
	}
	return &network, nil
}

func JsonEncoder(payload interface{}) (io.Reader, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("json Marshal: %s", err)
	}
	buf := bytes.NewBuffer(data)
	return buf, nil
}
