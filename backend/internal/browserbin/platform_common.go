package browserbin

import "runtime"

func runtimeKey() string { return runtime.GOOS + "/" + runtime.GOARCH }
