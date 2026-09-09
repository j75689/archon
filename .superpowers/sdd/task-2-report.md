**Status:** DONE
Commits: `0660d1d feat(lang): extract first-party go import graphs`
One-line test summary: `go test ./internal/lang/... -v` and `go test ./...` passed.
Concerns: None.
Report path: `.superpowers/sdd/task-2-report.md`

## Additional Test Evidence

```text
$ go test ./internal/lang/golang -v
Go test: 8 passed in 1 packages
```

```text
$ go test ./...
Go test: 12 passed in 3 packages
```
