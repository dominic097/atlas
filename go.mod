module github.com/MsysTechnologiesllc/aziron-atlas

go 1.23

// NOTE: the skeleton intentionally depends ONLY on stdlib + cobra so `go build ./...`
// works with zero CGO and zero heavy deps. The real engine adds (behind build tags):
//   github.com/tree-sitter/go-tree-sitter + 7 grammars   (CGO parser)
//   github.com/blevesearch/bleve/v2                       (BM25 lexical index)
//   github.com/mattn/go-sqlite3 / modernc.org/sqlite      (local StorageDriver)
//   github.com/jmoiron/sqlx + github.com/lib/pq           (hosted StorageDriver, -tags hosted)
//   github.com/pgvector/pgvector-go                       (optional VectorStore, -tags hosted)
//   github.com/google/uuid, go.uber.org/zap, gorilla/mux
// See docs/ARCHITECTURE.md.

require github.com/spf13/cobra v1.8.1

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
)
