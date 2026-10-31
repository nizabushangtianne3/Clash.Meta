package orderedmap

import (
	"bytes"
	"fmt"

	"go.yaml.in/yaml/v3"
)

var (
	_ yaml.Marshaler   = &OrderedMap[int, any]{}
	_ yaml.Unmarshaler = &OrderedMap[int, any]{}
)

// MarshalYAML implements the yaml.Marshaler interface.
func (om *OrderedMap[K, V]) MarshalYAML() (interface{}, error) {
	if om == nil {
		return []byte("null"), nil
	}

	items := make([]interface{}, 0, om.Len()*2)
	for pair := om.Oldest(); pair != nil; pair = pair.Next() {
		items = append(items, pair.Key, pair.Value)
	}

	var buffer bytes.Buffer
	encoder := yaml.NewEncoder(&buffer)
	if err := encoder.Encode(items); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}

	var sequence yaml.Node
	if err := yaml.Unmarshal(buffer.Bytes(), &sequence); err != nil {
		return nil, err
	}
	if len(sequence.Content) != 1 || sequence.Content[0].Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("ordered map produced %v instead of a YAML sequence", sequence.Content[0].Kind)
	}

	return &yaml.Node{
		Kind:    yaml.MappingNode,
		Content: sequence.Content[0].Content,
	}, nil
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (om *OrderedMap[K, V]) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("pipeline must contain YAML mapping, has %v", value.Kind)
	}

	if om.list == nil {
		om.initialize(0)
	}

	for index := 0; index < len(value.Content); index += 2 {
		var key K
		var val V

		if err := value.Content[index].Decode(&key); err != nil {
			return err
		}
		if err := value.Content[index+1].Decode(&val); err != nil {
			return err
		}

		om.Set(key, val)
	}

	return nil
}
