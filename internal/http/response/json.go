package response

import (
	"encoding/json"
	api "github.com/Pkhmv16/tripGO/internal/generated"
	"net/http"
)

func JSON(w http.ResponseWriter, r *http.Request, status int, contentType string, value any) {
	body, err := json.Marshal(value)
	if err != nil {
		status = http.StatusInternalServerError
		contentType = "application/problem+json"
		body, _ = json.Marshal(api.Problem{
			Type:     "https://tripgo.example/problems/internal-error",
			Title:    "Internal server error",
			Status:   int32(status),
			Code:     "internal_error",
			Detail:   new("Failed to encode response"),
			Instance: new(r.URL.Path),
		})
	}

	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	w.Write(body)
}
