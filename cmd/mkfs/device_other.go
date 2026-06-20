//go:build !darwin
package main

import "os"

func getDeviceSizePlatform(file *os.File) (int64, bool) {
	return 0, false
}
