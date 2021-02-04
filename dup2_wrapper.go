// +build !arm
// +build !arm64

package main

import (
	"syscall"
)

func Dup2(oldfd int, newfd int) {
	syscall.Dup2(oldfd, newfd)
}
