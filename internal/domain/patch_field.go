package domain

import "encoding/json"

// PatchField is a JSON field that distinguishes omitted keys from explicit null/values.
type PatchField[T any] struct {
	Set   bool
	Value T
}

func (p *PatchField[T]) UnmarshalJSON(data []byte) error {
	p.Set = true
	if string(data) == "null" {
		var zero T
		p.Value = zero
		return nil
	}
	return json.Unmarshal(data, &p.Value)
}
