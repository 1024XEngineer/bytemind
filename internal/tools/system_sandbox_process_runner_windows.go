//go:build windows

package tools

import (
	"errors"
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
)

func runCommandWithWindowsJobObject(cmd *exec.Cmd) error {
	if cmd == nil {
		return errors.New("command is required")
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		_ = cmd.Wait()
		return err
	}
	defer windows.CloseHandle(job)

	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)),
		uint32(unsafe.Sizeof(limits)),
	); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return err
	}

	processHandle, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_INFORMATION,
		false,
		uint32(cmd.Process.Pid),
	)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return err
	}
	defer windows.CloseHandle(processHandle)

	if err := windows.AssignProcessToJobObject(job, processHandle); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return err
	}

	return cmd.Wait()
}
