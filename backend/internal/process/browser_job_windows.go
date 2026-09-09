//go:build windows

package process

import (
	"fmt"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Assign the suspended launcher before its first instruction, so even a
// bootstrapper's descendants belong to this particular browser lifetime.
func containWindowsChild(child *Started) error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		if !ok {
			windows.CloseHandle(job)
		}
	}()
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE}}
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return err
	}
	handle, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(child.Pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	if err = windows.AssignProcessToJobObject(job, handle); err != nil {
		return err
	}
	threads, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(threads)
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	resumed := false
	for err = windows.Thread32First(threads, &entry); err == nil; err = windows.Thread32Next(threads, &entry) {
		if entry.OwnerProcessID != uint32(child.Pid) {
			continue
		}
		thread, openErr := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if openErr != nil {
			return openErr
		}
		_, resumeErr := windows.ResumeThread(thread)
		windows.CloseHandle(thread)
		if resumeErr != nil {
			return resumeErr
		}
		resumed = true
	}
	if !resumed {
		return fmt.Errorf("could not resume the contained browser process")
	}
	var mu sync.Mutex
	released := false
	child.verifyOwned = func(processHandle uintptr) error {
		mu.Lock()
		defer mu.Unlock()
		if released {
			return fmt.Errorf("browser scope already closed")
		}
		var member int32
		result, _, err := windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob").Call(processHandle, uintptr(job), uintptr(unsafe.Pointer(&member)))
		if result == 0 {
			return err
		}
		if member == 0 {
			return fmt.Errorf("browser process is not in Surf's owned job")
		}
		return nil
	}
	child.killOwned = func() error {
		mu.Lock()
		defer mu.Unlock()
		if released {
			return fmt.Errorf("browser already exited")
		}
		return windows.TerminateJobObject(job, 1)
	}
	child.waitOwned = func(timeout time.Duration) error {
		deadline := time.Now().Add(timeout)
		for {
			mu.Lock()
			if released {
				mu.Unlock()
				return nil
			}
			var accounting struct {
				TotalUserTime, TotalKernelTime, ThisPeriodTotalUserTime, ThisPeriodTotalKernelTime int64
				TotalPageFaultCount, TotalProcesses, ActiveProcesses, TotalTerminatedProcesses     uint32
			}
			err := windows.QueryInformationJobObject(job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&accounting)), uint32(unsafe.Sizeof(accounting)), nil)
			mu.Unlock()
			if err != nil {
				return err
			}
			if accounting.ActiveProcesses == 0 {
				return nil
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("browser helpers are still closing")
			}
			time.Sleep(25 * time.Millisecond)
		}
	}
	child.releaseOwned = func() {
		mu.Lock()
		defer mu.Unlock()
		if !released {
			windows.CloseHandle(job)
			released = true
		}
	}
	ok = true
	return nil
}
