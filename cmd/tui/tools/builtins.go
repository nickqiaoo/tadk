package tools

import (
	adktool "github.com/nickqiaoo/tadk/tool"
	"github.com/nickqiaoo/tadk/tool/bash"
	"github.com/nickqiaoo/tadk/tool/editfile"
	"github.com/nickqiaoo/tadk/tool/findfile"
	"github.com/nickqiaoo/tadk/tool/grepfile"
	"github.com/nickqiaoo/tadk/tool/interrupt"
	"github.com/nickqiaoo/tadk/tool/readfile"
	"github.com/nickqiaoo/tadk/tool/writefile"
)

func Builtins() []adktool.Tool {
	return []adktool.Tool{
		interrupt.WithApproval(bashtool.New(), "Run a shell command in the local workspace?"),
		readfiletool.New(),
		interrupt.WithApproval(writefiletool.New(), "Write or overwrite a local file?"),
		interrupt.WithApproval(editfiletool.New(), "Edit a local file?"),
		grepfiletool.New(),
		findfiletool.New(),
	}
}
