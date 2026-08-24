//go:build windows

package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// superviseChildren binds this process to a Job Object configured to kill every
// process in the job when its last handle closes. The child Chrome launched
// afterward inherits the job, so an uncatchable TerminateProcess of this process
// — which delivers no signal and runs no defer — still reaps the whole browser
// tree at the OS level. This is the Windows counterpart to the signal-driven
// cleanup that handles the catchable paths on all platforms.
//
// The job handle is intentionally never closed: it must stay open for the
// process lifetime, because it is the closing of that last handle (on process
// exit, for any reason) that triggers kill-on-close.
func superviseChildren() error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		_ = windows.CloseHandle(job)
		return err
	}
	if err := windows.AssignProcessToJobObject(job, windows.CurrentProcess()); err != nil {
		_ = windows.CloseHandle(job)
		return err
	}
	return nil
}
