// adkgo is a CLI tool to help deploy and test an ADK application.
package main

import (
	_ "github.com/nickqiaoo/tadk/cmd/adkgo/internal/deploy/cloudrun"
	"github.com/nickqiaoo/tadk/cmd/adkgo/internal/root"
)

func main() {
	root.Execute()
}
