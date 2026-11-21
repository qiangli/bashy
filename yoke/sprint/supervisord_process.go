package main

import (
	"encoding/json"
)

func fingerprint(argv, env []string, dir string) string {
	b, _ := json.Marshal([]any{argv, env, dir})
	return string(b)
}
