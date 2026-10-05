//go:build !deckoldschema

package store

// supportedSchema is the newest schema this binary opens. Shipped builds,
// `go build ./...` and `go test ./...` always see SchemaVersion; see
// schema_old_hook.go for the test-only build that lowers it.
const supportedSchema = SchemaVersion
