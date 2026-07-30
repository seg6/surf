package chromium

import "runtime"

func runtimeKey() string { return runtime.GOOS + "/" + runtime.GOARCH }
