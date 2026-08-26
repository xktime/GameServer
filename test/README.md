# Tests

Behavior tests run with the default `go test ./...` command. Repeat the Actor concurrency suite with `go test -race -count=20 ./common/base/actor` when changing mailbox or lifecycle semantics.
