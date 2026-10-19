// Package api is the HTTP client of the public OpenViCloud /v1 contract. client.gen.go is generated from
// openapi-v1.json (copied from the OpenViCloud API by scripts/sync-spec.sh) and never edited by hand; CI
// regenerates it and fails on a diff.
package api

//go:generate go tool oapi-codegen -config oapi-codegen.yaml openapi-v1.json
