module github.com/tuzig/webexec

go 1.16

// replace github.com/pion/webrtc/v3 => /Users/daonb/src/webrtc

// replace github.com/pion/datachannel => /Users/daonb/src/datachannel

require (
	git.rootprojects.org/root/go-gitver/v2 v2.0.2
	github.com/creack/pty v1.1.11
	github.com/dchest/uniuri v0.0.0-20200228104902-7aecb25e1fe5
	github.com/gorilla/websocket v1.4.2
	github.com/hinshun/vt10x v0.0.0-20201217012337-52c1408d37d6
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0
	github.com/pelletier/go-toml v1.9.3
	github.com/pion/webrtc/v3 v3.1.49
	github.com/riywo/loginshell v0.0.0-20200815045211-7d26008be1ab
	github.com/rs/cors v1.7.0
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/shirou/gopsutil/v3 v3.21.10
	github.com/stretchr/testify v1.8.0
	github.com/urfave/cli/v2 v2.3.0
	go.uber.org/fx v1.19.2
	go.uber.org/zap v1.23.0
	golang.org/x/crypto v0.0.0-20221010152910-d6f0a8c073c2
	golang.org/x/net v0.7.0 // indirect
	golang.org/x/sys v0.5.0
	gopkg.in/natefinch/lumberjack.v2 v2.0.0
)
