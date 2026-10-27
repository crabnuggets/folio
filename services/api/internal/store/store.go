// Package store commits an owner aggregate and its retry response atomically.
package store

import (
	"context"
	"encoding/json"
	"github.com/crabnuggets/folio/services/api/internal/domain"
	"strconv"
)

type Result struct {
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body"`
	ETag   string          `json:"etag"`
}

// JSON encodes a retryable response and adds a strong ETag when version is positive.
// JSON encoding errors are returned to the caller.
func JSON(status int, body any, version int64) (Result, error) {
	b, e := json.Marshal(body)
	r := Result{Status: status, Body: b}
	if version > 0 {
		r.ETag = fmtETag(version)
	}
	return r, e
}
// fmtETag quotes a decimal resource version for use as a strong ETag.
func fmtETag(v int64) string { return `"` + strconv.FormatInt(v, 10) + `"` }

type Store interface {
	Read(context.Context, string) (*domain.State, error)
	Execute(context.Context, string, string, string, func(*domain.State) (Result, error)) (Result, bool, error)
	Ping(context.Context) error
	Close()
}
