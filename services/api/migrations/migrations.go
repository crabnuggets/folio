package migrations

import _ "embed"

//go:embed 001_owner_aggregate.sql
var initial string

//go:embed 002_retry_response_bytes.sql
var responseBytes string

type Migration struct {
	Version int64
	SQL     string
}

var All = []Migration{{1, initial}, {2, responseBytes}}
