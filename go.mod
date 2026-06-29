module github.com/dominic097/atlas

go 1.25.0

// NOTE: the skeleton intentionally depends ONLY on stdlib + cobra so `go build ./...`
// works with zero CGO and zero heavy deps. The real engine adds (behind build tags):
//   github.com/tree-sitter/go-tree-sitter + 7 grammars   (CGO parser)
//   github.com/blevesearch/bleve/v2                       (BM25 lexical index)
//   github.com/mattn/go-sqlite3 / modernc.org/sqlite      (local StorageDriver)
//   github.com/jmoiron/sqlx + github.com/lib/pq           (hosted StorageDriver, -tags hosted)
//   github.com/pgvector/pgvector-go                       (optional VectorStore, -tags hosted)
//   github.com/google/uuid, go.uber.org/zap, gorilla/mux
// See docs/ARCHITECTURE.md.

require (
	github.com/UserNobody14/tree-sitter-dart v0.0.0-20260520003023-a9bdfa3db2fb
	github.com/blevesearch/bleve/v2 v2.6.0
	github.com/fsnotify/fsnotify v1.10.1
	github.com/google/uuid v1.6.0
	github.com/lib/pq v1.10.9
	github.com/spf13/cobra v1.10.2
	github.com/tree-sitter-grammars/tree-sitter-kotlin v1.1.0
	github.com/tree-sitter-grammars/tree-sitter-lua v0.5.0
	github.com/tree-sitter-grammars/tree-sitter-zig v1.1.2
	github.com/tree-sitter/go-tree-sitter v0.25.0
	github.com/tree-sitter/tree-sitter-c v0.24.2
	github.com/tree-sitter/tree-sitter-c-sharp v0.23.5
	github.com/tree-sitter/tree-sitter-cpp v0.23.4
	github.com/tree-sitter/tree-sitter-elixir v0.3.5
	github.com/tree-sitter/tree-sitter-java v0.23.5
	github.com/tree-sitter/tree-sitter-javascript v0.25.0
	github.com/tree-sitter/tree-sitter-julia v0.23.1
	github.com/tree-sitter/tree-sitter-php v0.24.2
	github.com/tree-sitter/tree-sitter-python v0.25.0
	github.com/tree-sitter/tree-sitter-ruby v0.23.1
	github.com/tree-sitter/tree-sitter-rust v0.24.2
	github.com/tree-sitter/tree-sitter-scala v0.26.0
	github.com/tree-sitter/tree-sitter-typescript v0.23.2
	golang.org/x/sync v0.21.0
	golang.org/x/tools v0.46.0
	modernc.org/sqlite v1.53.0
)

require (
	github.com/RoaringBitmap/roaring/v2 v2.14.5 // indirect
	github.com/bits-and-blooms/bitset v1.24.2 // indirect
	github.com/blevesearch/bleve_index_api v1.3.11 // indirect
	github.com/blevesearch/geo v0.2.5 // indirect
	github.com/blevesearch/go-faiss v1.1.0 // indirect
	github.com/blevesearch/go-porterstemmer v1.0.3 // indirect
	github.com/blevesearch/gtreap v0.1.1 // indirect
	github.com/blevesearch/mmap-go v1.2.0 // indirect
	github.com/blevesearch/scorch_segment_api/v2 v2.4.7 // indirect
	github.com/blevesearch/segment v0.9.1 // indirect
	github.com/blevesearch/snowballstem v0.9.0 // indirect
	github.com/blevesearch/upsidedown_store_api v1.0.2 // indirect
	github.com/blevesearch/vellum v1.2.0 // indirect
	github.com/blevesearch/zapx/v11 v11.4.3 // indirect
	github.com/blevesearch/zapx/v12 v12.4.3 // indirect
	github.com/blevesearch/zapx/v13 v13.4.3 // indirect
	github.com/blevesearch/zapx/v14 v14.4.3 // indirect
	github.com/blevesearch/zapx/v15 v15.4.3 // indirect
	github.com/blevesearch/zapx/v16 v16.3.4 // indirect
	github.com/blevesearch/zapx/v17 v17.1.2 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/golang/snappy v1.0.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/json-iterator/go v0.0.0-20171115153421-f7279a603ede // indirect
	github.com/ledongthuc/pdf v0.0.0-20250511090121-5959a4027728 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-pointer v0.0.1 // indirect
	github.com/mschoch/smat v0.2.0 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	github.com/tree-sitter/tree-sitter-go v0.25.0 // indirect
	go.etcd.io/bbolt v1.4.0 // indirect
	golang.org/x/mod v0.37.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
	modernc.org/libc v1.73.4 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)

replace github.com/tree-sitter/tree-sitter-elixir => github.com/elixir-lang/tree-sitter-elixir v0.3.5
