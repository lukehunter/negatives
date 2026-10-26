//go:build windows

package archivesync

import (
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	jobOnce sync.Once
	job     windows.Handle // kept open for the life of the process, never closed
)

// killWithUs puts the process in a job object that Windows closes when
// archive-sync exits for any reason, killing whatever is still in it. If
// the job object can't be set up, robocopy runs as a plain child process:
// a robocopy left running after archive-sync is killed is the only
// consequence, and quarantine's re-check before moving guards against that.
func killWithUs(pid int) {
	jobOnce.Do(func() {
		h, err := windows.CreateJobObject(nil, nil)
		if err != nil {
			return
		}
		var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
		info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
		if _, err := windows.SetInformationJobObject(h, windows.JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
			windows.CloseHandle(h)
			return
		}
		job = h
	})
	if job == 0 {
		return
	}
	p, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return
	}
	defer windows.CloseHandle(p)
	_ = windows.AssignProcessToJobObject(job, p)
}
