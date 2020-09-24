# A Tiny PIDFile Library for Golang

[![Build Status](https://travis-ci.org/mingcheng/pidfile.svg?branch=master)](https://travis-ci.org/mingcheng/pidfile) [![Go Report Card](https://goreportcard.com/badge/github.com/mingcheng/pidfile)](https://goreportcard.com/report/github.com/mingcheng/pidfile)

This package provides structure and helper functions to create and remove PID file.
PIDFile is a file used to store the process ID of a running process.

For more information and documents, visit https://godoc.org/github.com/mingcheng/pidfile.go

## Feature

* Support on muti-system (Linux, macOS, Windows and FreeBSD)
* With all full tested

## Usage

To usage this package is simple, here is an example:

```golang
import	"github.com/mingcheng/pidfile"

var pidFilePath = "/var/run/my.pid"
if pid, err := pidfile.New(pidFilePath); err != nil {
  log.Panic(err)
} else {
  fmt.Println(pid)
  defer pid.Remove()
}
```

## Feedback

If you have any suggest, sending me via email to `echo bWluZ2NoZW5nQG91dGxvb2suY29tCg== | base64 -D`, with huge thanks.

`- eof -`
