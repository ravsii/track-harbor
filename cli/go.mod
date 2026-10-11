module trackharbor/cli

go 1.26.4

require (
	github.com/clipperhouse/uax29/v2 v2.2.0
	github.com/jmoiron/sqlx v1.4.0
	github.com/klauspost/compress v1.18.6
	github.com/mattn/go-runewidth v0.0.31
	github.com/mattn/go-sqlite3 v1.14.46
	github.com/spf13/cobra v1.10.2
	go.uber.org/zap v1.28.0
	golang.org/x/term v0.47.0
	gopkg.in/yaml.v3 v3.0.1
	trackharbor v0.0.0
)

replace trackharbor => ../trackharbor

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/kr/pretty v0.1.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	go.uber.org/multierr v1.10.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	gopkg.in/check.v1 v1.0.0-20180628173108-788fd7840127 // indirect
)
