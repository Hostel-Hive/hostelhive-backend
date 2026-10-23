package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"time"
	"unicode/utf8"

	identity "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
)

type OwnProfileEditService interface {
	Patch(context.Context, string, dto.OwnProfilePatch) (d.OwnProfile, error)
}

type OwnProfileEditAPI struct {
	service OwnProfileEditService
	timeout time.Duration
}

func NewOwnProfileEditAPI(service OwnProfileEditService, timeout time.Duration) *OwnProfileEditAPI {
	return &OwnProfileEditAPI{service: service, timeout: timeout}
}

func (a *OwnProfileEditAPI) Patch(w http.ResponseWriter, r *http.Request) {
	actor, ok := middleware.AccountFromContext(r.Context())
	if !ok {
		reject(w, 401, "unauthorized")
		return
	}
	if !actor.IsActive || actor.Role != identity.RoleStudent {
		reject(w, 403, "forbidden")
		return
	}
	if r.URL.RawQuery != "" {
		failure(w, d.ErrInvalid)
		return
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		reject(w, 415, "unsupported_media_type")
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16*1024))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			reject(w, 413, "payload_too_large")
		} else {
			failure(w, d.ErrInvalid)
		}
		return
	}
	input, err := decodeOwnPatch(raw)
	if err != nil {
		failure(w, d.ErrInvalid)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	result, err := a.service.Patch(ctx, actor.FirebaseUID, input)
	if err != nil {
		failure(w, err)
		return
	}
	reply(w, 200, result)
}

func decodeOwnPatch(raw []byte) (dto.OwnProfilePatch, error) {
	var input dto.OwnProfilePatch
	raw = bytes.TrimSpace(raw)
	if !utf8.Valid(raw) || len(raw) == 0 || raw[0] != '{' {
		return input, d.ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := uniqueJSONValue(decoder, 0); err != nil {
		return input, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return input, d.ErrInvalid
	}
	decoder = json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return input, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || len(fields) == 0 {
		return input, d.ErrInvalid
	}
	for name, value := range fields {
		if name != "contact_phone" && name != "guardians" {
			return input, d.ErrInvalid
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return input, d.ErrInvalid
		}
		if name == "guardians" {
			var gs []map[string]json.RawMessage
			if err := json.Unmarshal(value, &gs); err != nil {
				return input, err
			}
			for _, guardian := range gs {
				for field := range guardian {
					if field != "name" && field != "relationship" && field != "contact_phone" {
						return input, d.ErrInvalid
					}
				}
			}
		}
	}
	return input, nil
}

// Reject duplicate fields (including nested guardians), excessive nesting and
// malformed JSON before decoding the allowlisted DTO. No last-key-wins parsing.
func uniqueJSONValue(decoder *json.Decoder, depth int) error {
	if depth > 32 {
		return d.ErrInvalid
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	if delim != '{' && delim != '[' {
		return d.ErrInvalid
	}
	seen := map[string]bool{}
	for decoder.More() {
		if delim == '{' {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return d.ErrInvalid
			}
			seen[name] = true
		}
		if err := uniqueJSONValue(decoder, depth+1); err != nil {
			return err
		}
	}
	end, err := decoder.Token()
	if err != nil {
		return err
	}
	if (delim == '{' && end != json.Delim('}')) || (delim == '[' && end != json.Delim(']')) {
		return d.ErrInvalid
	}
	return nil
}
