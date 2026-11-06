package create

import (
	"bytes"
	"encoding/json"
	"math"

	"github.com/Pkhmv16/tripGO/internal/domain/trip"
	api "github.com/Pkhmv16/tripGO/internal/generated"
)

const (
	bodyField = "body"
)

func validateCreateTripFields(raw json.RawMessage) error {
	fields, err := requireFields(raw, "", "user_id", "driver_id", "start_point", "end_point", "price")
	if err != nil {
		return err
	}
	for _, name := range []string{"start_point", "end_point"} {
		if _, err := requireFields(fields[name], name, "latitude", "longitude"); err != nil {
			return err
		}
	}
	return nil
}

func requireFields(raw json.RawMessage, path string, names ...string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		field := path
		if field == "" {
			field = bodyField
		}
		return nil, &trip.ValidationError{Field: field, Reason: "must be a JSON object"}
	}
	fieldPath := func(name string) string {
		if path == "" {
			return name
		}
		return path + "." + name
	}
	allowed := make(map[string]struct{}, len(names))
	for _, name := range names {
		allowed[name] = struct{}{}
		value, exists := fields[name]
		if !exists || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, &trip.ValidationError{Field: fieldPath(name), Reason: "is required and cannot be null"}
		}
	}
	for name := range fields {
		if _, ok := allowed[name]; !ok {
			return nil, &trip.ValidationError{Field: fieldPath(name), Reason: "unknown field"}
		}
	}
	return fields, nil
}

func validateCoordinate(field string, point api.Coordinates) error {
	if math.IsNaN(point.Latitude) || point.Latitude < -90 || point.Latitude > 90 {
		return &trip.ValidationError{Field: field + ".latitude", Reason: "must be between -90 and 90"}
	}
	if math.IsNaN(point.Longitude) || point.Longitude < -180 || point.Longitude > 180 {
		return &trip.ValidationError{Field: field + ".longitude", Reason: "must be between -180 and 180"}
	}
	return nil
}
