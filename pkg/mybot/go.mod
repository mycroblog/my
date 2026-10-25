module github.com/mycroblog/my/pkg/mybot

go 1.27.3

require github.com/mycroblog/my/pkg/mykit v0.0.0-00010101000000-000000000000

require (
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/oapi-codegen/runtime v1.7.0 // indirect
)

// mybot is built and released with the pkg/mykit of the same commit (ADR-0094 §1).
replace github.com/mycroblog/my/pkg/mykit => ../mykit
