//go:build !unix

package main

import "os"

func openWorkerStatusFile(path string) (*os.File, error) {
	return os.Open(path)
}
