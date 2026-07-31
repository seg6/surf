package process

import (
	"os"
	"testing"
)

func TestRunningRecognizesCurrentAndMissingProcess(t *testing.T) {
	if !Running(os.Getpid()) {
		t.Fatal("current process is not running")
	}
	if Running(1<<30 - 1) {
		t.Fatal("impossible process id reported as running")
	}
}
