// Package pidfile provides structure and helper functions to create and remove
// PID file. A PID file is usually a file used to store the process ID of a
// running process.
//
// @ref https://github.com/moby/moby/tree/master/pkg/pidfile
package pidfile

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Common error for pidfile package
var (
	ErrProcessRunning = errors.New("process is running")
	ErrFileStale      = errors.New("pidfile exists but process is not running")
	ErrFileInvalid    = errors.New("pidfile has invalid contents")
)

// PIDFile is a file used to store the process ID of a running process.
type PIDFile struct {
	path string
	pid  int
}

func Open(path string) (*PIDFile, error) {
	var file = PIDFile{
		path: path,
	}
	pid, err := file.Read()
	file.pid = pid
	if err != nil {
		return nil, err
	}

	return &file, nil
}

// New creates a PIDfile using the specified path.
func New(path string) (*PIDFile, error) {
	var file = PIDFile{
		path: path,
		pid:  os.Getpid(),
	}

	if pid, err := file.Read(); err == nil && processExists(pid) {
		return nil, ErrProcessRunning
	}

	if err := file.Write(); err != nil {
		return nil, err
	}

	return &file, nil
}

// Remove the PIDFile.
func (file PIDFile) Remove() error {
	return os.Remove(file.path)
}

// Read the PIDFile content.
func (file PIDFile) Read() (int, error) {
	if contents, err := ioutil.ReadFile(file.path); err != nil {
		return 0, err
	} else {
		pid, err := strconv.Atoi(strings.TrimSpace(string(contents)))
		if err != nil {
			return 0, ErrFileInvalid
		}
		file.pid = pid
		return pid, nil
	}
}

// Write writes a pidfile, returning an error
// if the process is already running or pidfile is orphaned
func (file PIDFile) Write() error {
	pid := os.Getpid()
	// Check for existing pid
	if oldPid, err := file.Read(); err != nil && !os.IsNotExist(err) {
		return err
	} else if err == nil {
		// We have a pid
		if processExists(oldPid) {
			return ErrProcessRunning
		}
	}

	// Note MkdirAll returns nil if a directory already exists
	if err := os.MkdirAll(filepath.Dir(file.path), os.FileMode(0700)); err != nil {
		return err
	}

	// We're clear to (over)write the file
	return ioutil.WriteFile(file.path, []byte(fmt.Sprintf("%d\n", pid)), 0600)
}

// Detect whether is process is running.
func (file PIDFile) Running() bool {
	return processExists(file.pid)
}
