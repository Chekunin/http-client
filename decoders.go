package http_client

import (
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
)

func GobDecoder(reader io.Reader, res interface{}) error {
	dec := gob.NewDecoder(reader)
	if err := dec.Decode(res); err != nil {
		return fmt.Errorf("gob dec.Decode: %s", err)
	}
	return nil
}

func JsonDecoder(reader io.Reader, res interface{}) error {
	dec := json.NewDecoder(reader)
	if err := dec.Decode(res); err != nil {
		return fmt.Errorf("json dec.Decode: %s", err)
	}
	return nil
}
