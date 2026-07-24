package normalize

import (
	"strconv"
	"strings"
)

func strP(m map[string]any, key string) *string {
	if v, ok := m[key].(string); ok && v != "" {
		return &v
	}
	return nil
}

func firstStrP(m map[string]any, keys ...string) *string {
	for _, k := range keys {
		if p := strP(m, k); p != nil {
			return p
		}
	}
	return nil
}

// strAnyP stringifies a value that may be number or string (e.g. ExposureTime "1/120").
func strAnyP(m map[string]any, key string) *string {
	switch v := m[key].(type) {
	case string:
		if v != "" {
			return &v
		}
	case float64:
		s := strconv.FormatFloat(v, 'g', -1, 64)
		return &s
	}
	return nil
}

func floatP(m map[string]any, key string) *float64 {
	switch v := m[key].(type) {
	case float64:
		return &v
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return &f
		}
	}
	return nil
}

func firstFloatP(m map[string]any, keys ...string) *float64 {
	for _, k := range keys {
		if p := floatP(m, k); p != nil {
			return p
		}
	}
	return nil
}

func intP(m map[string]any, key string) *int {
	if p := floatP(m, key); p != nil {
		i := int(*p)
		return &i
	}
	return nil
}

func firstIntP(m map[string]any, keys ...string) *int {
	for _, k := range keys {
		if p := intP(m, k); p != nil {
			return p
		}
	}
	return nil
}

func intP64(m map[string]any, key string) *int64 {
	if p := floatP(m, key); p != nil {
		i := int64(*p)
		return &i
	}
	return nil
}

func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}

func parseInt64(s string) int64 {
	i, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return i
}

// parseFrameRate parses ffprobe's "30000/1001" fraction.
func parseFrameRate(s string) float64 {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) == 2 {
		num := parseFloat(parts[0])
		den := parseFloat(parts[1])
		if den > 0 {
			return num / den
		}
	}
	return parseFloat(s)
}
