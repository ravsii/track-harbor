module trackharbor/server

go 1.26.4

require (
	github.com/mattn/go-sqlite3 v1.14.46
	gopkg.in/yaml.v3 v3.0.1
	trackharbor v0.0.0
)

require (
	github.com/klauspost/compress v1.18.6 // indirect
	go.uber.org/multierr v1.10.0 // indirect
	go.uber.org/zap v1.28.0 // indirect
)

replace trackharbor => ../trackharbor
