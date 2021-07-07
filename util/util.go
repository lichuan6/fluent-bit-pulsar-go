package util

import (
	"encoding/json"
	"fmt"
	"strings"
)

// AddMessage adds fluent-bit data as pulsar message
// parameter key is the pulsar topic(i.e tenant/namespace/topic)
// parameter value is the data of json encoded string
func AddMessage(m map[string][]string, key string, value string) {
	if value == "" {
		return
	}
	if _, ok := m[key]; !ok {
		m[key] = make([]string, 0)
	}
	m[key] = append(m[key], value)
}

func ParseK8sNamespaceFromTag(tag string) string {
	splits := strings.Split(tag, ".")
	if len(splits) < 2 {
		return ""
	}
	return splits[1]
}

func buildMapFromRecord(record map[interface{}]interface{}) map[string]interface{} {
	m := make(map[string]interface{})
	for k, v := range record {
		key := fmt.Sprintf("%s", k)
		if key == "kubernetes" {
			continue
		}
		m[key] = v
	}

	return m
}

func flatten(m map[string]interface{}) map[string]interface{} {
	o := make(map[string]interface{})
	for k, v := range m {
		switch child := v.(type) {
		case map[string]interface{}:
			nm := flatten(child)
			for nk, nv := range nm {
				o[k+"."+nk] = nv
			}
		default:
			o[k] = v
		}
	}
	return o
}

func FlattenRecordMap(record map[string]interface{}) map[string]interface{} {
	v, ok := record["log"]
	if !ok {
		// log is not in record, return flatten record
		return flatten(record)
	}
	// try to unmarshal log's value
	m := make(map[string]interface{})
	var b []byte
	switch v := v.(type) {
	case []uint8:
		b = v
	case string:
		b = []byte(v)
	default:
		b = nil
	}
	if b == nil {
		return flatten(record)
	}

	if err := json.Unmarshal(b, &m); err != nil {
		// something wrong happens, do not unmarshal
		return flatten(record)
	}
	// we can unmarshal log's value into map
	m, err := data2map(b)
	if err != nil {
		// cannot parse log's value to map, use raw
	} else {
		// add new unmasharled map to result map, and keep the log(raw data)
		for k, v := range m {
			record[k] = v
		}
	}
	return flatten(record)
}

func Convert(m map[interface{}]interface{}) map[string]interface{} {
	o := make(map[string]interface{})

	for k, v := range m {
		key := fmt.Sprintf("%s", k)
		switch child := v.(type) {
		case map[interface{}]interface{}:
			nm := Convert(child)
			o[key] = nm
		case []byte:
			o[key] = fmt.Sprintf("%s", v)
		default:
			// o[key] = fmt.Sprintf("%s", v)
			// o[key] = v
			o[key] = fmt.Sprintf("%v", v)
		}
	}
	return o
}

func data2map(data []byte) (map[string]interface{}, error) {
	m := make(map[string]interface{})
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}
