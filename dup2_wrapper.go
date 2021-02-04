// +build !arm freebsd
// +build !arm,!arm64 !linux

package main

import "syscall"

func Dup2(oldfd int, newfd int) {
	syscall.Dup2(oldfd, newfd)
}
