package main

import (
	"fmt"
	"time"
)

func normalizeDBValue(v interface{}) interface{} {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case []byte:
		return string(val)
	case time.Time:
		return val.Format(time.RFC3339)
	case fmt.Stringer:
		return val.String()
	default:
		return val
	}
}

func emptyRows() []map[string]interface{} {
	return make([]map[string]interface{}, 0)
}
