module github.com/gallowaysoftware/worldsmith

go 1.26.3

require (
	github.com/gallowaysoftware/vibe v0.6.2
	github.com/spf13/cobra v1.10.2
	gopkg.in/yaml.v3 v3.0.1
)

require (
	connectrpc.com/connect v1.19.2 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	golang.org/x/sys v0.36.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

// In-tree vibe checkout — same pattern fake-crime / iitn / textbook
// follow. Drop the replace once worldsmith stops depending on
// post-v0.5.1 vamp features (mix.Metadata + activate/doctor are the
// load-bearing ones today).
replace github.com/gallowaysoftware/vibe => ../vibe
