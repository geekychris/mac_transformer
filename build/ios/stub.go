//go:build !ios

package main

// Stub main so `go build ./...` succeeds on non-iOS builds. The real iOS
// entry point lives in main_ios.go under the `ios` build tag.
func main() {}
