// +build !windows
// +build arm arm64

package main

import (
	"syscall"
)

func Dup2(oldfd int, newfd int) {
	syscall.Dup3(oldfd, newfd, 0)
}
