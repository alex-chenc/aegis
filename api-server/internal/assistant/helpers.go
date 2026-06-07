package assistant

import (
	"encoding/json"
	"time"

	"gorm.io/datatypes"
)

// timeNow returns current time (overridable for testing)
var timeNow = func() time.Time { return time.Now() }

// timeSince returns duration in milliseconds since the given time
func timeSince(start time.Time) int64 {
	return time.Since(start).Milliseconds()
}

// mustMarshalJSON marshals value to JSON bytes, returns "{}" on nil/error
func mustMarshalJSON(v interface{}) datatypes.JSON {
	if v == nil {
		return datatypes.JSON("{}")
	}
	b, err := json.Marshal(v)
	if err != nil {
		return datatypes.JSON("{}")
	}
	return datatypes.JSON(b)
}

// unmarshalJSON unmarshals JSON bytes to map
func unmarshalJSON(data datatypes.JSON) map[string]interface{} {
	if len(data) == 0 {
		return nil
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}
	return result
}

// marshalToString marshals value to JSON string
func marshalToString(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
