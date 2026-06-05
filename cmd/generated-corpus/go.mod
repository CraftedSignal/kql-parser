module github.com/craftedsignal/kql-parser/cmd/generated-corpus

go 1.25.0

require (
	github.com/craftedsignal/kql-parser v0.0.0
	github.com/craftedsignal/sigma-parser v0.0.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/antlr4-go/antlr/v4 v4.13.1 // indirect
	golang.org/x/exp v0.0.0-20260508232706-74f9aab9d74a // indirect
)

replace github.com/craftedsignal/kql-parser => ../..

replace github.com/craftedsignal/sigma-parser => ../../../sigma-parser
