//go:build !unix

package config

// chownLike is a no-op on platforms without Unix file ownership.
func chownLike(target, path string) {}
