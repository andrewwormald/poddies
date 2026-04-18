package orchestrator

import "os"

// osMkdirAll exists to keep orchestrator_test.go free of an os import
// just for MkdirAll (stylistic; each file explicit).
func osMkdirAll(path string, perm uint32) error {
	return os.MkdirAll(path, os.FileMode(perm))
}
