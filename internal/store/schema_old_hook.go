//go:build deckoldschema

package store

// oldSchemaVersion is the schema this test build claims to support, settable
// with -ldflags "-X github.com/n-orlov/deck/internal/store.oldSchemaVersion=N".
// The file is compiled in only under `-tags deckoldschema`, which no release
// or default build uses: it lets a test stand in for a hook command bound to
// an older deck without ever building a real old release (R204).
var oldSchemaVersion = "7"

var supportedSchema = parseSchemaOverride(oldSchemaVersion, SchemaVersion-1)
