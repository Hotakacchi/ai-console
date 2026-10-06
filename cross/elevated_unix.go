//go:build !windows

package main

import "os"

// ルミ自体が root として動いているか
func isElevated() bool { return os.Geteuid() == 0 }
