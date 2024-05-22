//go:generate go run git.rootprojects.org/root/go-gitver/v2 --fail
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/coreos/go-semver/semver"
	"github.com/kardianos/osext"
	"github.com/pion/webrtc/v3"
	"github.com/tuzig/webexec/httpserver"
	"github.com/tuzig/webexec/peers"
	"github.com/tuzig/webexec/pidfile"
	"github.com/urfave/cli/v2"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/crypto/ssh/terminal"
	"gopkg.in/natefinch/lumberjack.v2"
)

type GHReleaseInfo struct {
	TagName string `json:"tag_name"`
}

var (
	// Logger is our global logger
	Logger *zap.SugaredLogger
	// generated by go-gitver
	commit  = "0000000"
	version = "UNRELEASED"
	date    = "0000-00-00T00:00:00+0000"
	// ErrAgentNotRunning is returned by commands that require a running agent
	ErrAgentNotRunning = errors.New("agent is not running")
	gotExitSignal      chan bool
	logWriter          io.Writer
	key                *KeyType
	cachedVersion      struct {
		version *semver.Version
		expire  time.Time
	}
	markerM sync.RWMutex
	// the id of the last marker used
	lastMarker = 0
)

func GetWelcome() string {
	msg := "Connected over WebRTC\r\n"
	note := getVersionNote()
	if note != "" {
		msg += "└─ " + note + "\r\n"
	}
	return msg
}

func getVersionNote() string {
	if cachedVersion.version == nil || cachedVersion.expire.After(time.Now()) {
		resp, err := http.Get("https://api.github.com/repos/tuzig/webexec/releases/latest")
		if err != nil {
			Logger.Warnf("Get version request failed: %s", err)
			return ""
		}
		defer resp.Body.Close()

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			Logger.Warnf("Could not read the latest version description: %s", err)
			return ""
		}

		var releaseInfo GHReleaseInfo
		err = json.Unmarshal(body, &releaseInfo)
		if err != nil {
			Logger.Warnf("Could not unmarshal the latest version body: %s", err)
			return ""
		}
		s := strings.TrimPrefix(releaseInfo.TagName, "v")
		cachedVersion.version, err = semver.NewVersion(s)
		if err != nil {
			Logger.Warnf("Could not parse the latest version: %s", err)
			return ""
		}
		cachedVersion.expire = time.Now().Add(time.Hour)
	}
	latestVersion := cachedVersion.version
	currentVersion, err := semver.NewVersion(version)
	if err != nil {
		return ""
	}
	if currentVersion.LessThan(*latestVersion) {
		return fmt.Sprintf("webexec version %s is available, please run `webexec upgrade`\n", latestVersion)
	}
	return ""
}

// PIDFIlePath return the path of the PID file
func PIDFilePath() string {
	return RunPath("webexec.pid")
}

// InitAgentLogger intializes an agent logger and sets the global Logger
func InitAgentLogger() *zap.SugaredLogger {
	// rotate the log file
	logWriter = &lumberjack.Logger{
		Filename:   Conf.logFilePath,
		MaxSize:    10, // megabytes
		MaxBackups: 3,
		MaxAge:     28, // days
	}
	w := zapcore.AddSync(logWriter)

	// TODO: use pion's logging
	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(zapcore.EncoderConfig{
			MessageKey:  "webexec",
			LevelKey:    "level",
			EncodeLevel: zapcore.CapitalLevelEncoder,
			TimeKey:     "time",
			EncodeTime:  zapcore.ISO8601TimeEncoder,
		}),
		w,
		Conf.logLevel,
	)
	logger := zap.New(core)
	defer logger.Sync()
	Logger = logger.Sugar()
	// redirect stderr
	e, _ := os.OpenFile(
		Conf.errFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	Dup2(int(e.Fd()), 2)
	return Logger
}

// InitDevLogger starts a logger for development
func InitDevLogger() *zap.SugaredLogger {
	zapConf := []byte(`{
		  "level": "debug",
		  "encoding": "console",
		  "outputPaths": ["stdout"],
		  "errorOutputPaths": ["stderr"],
		  "encoderConfig": {
		    "messageKey": "message",
		    "levelKey": "level",
		    "levelEncoder": "lowercase"
		  }
		}`)

	var cfg zap.Config
	if err := json.Unmarshal(zapConf, &cfg); err != nil {
		panic(err)
	}
	l, err := cfg.Build()
	Logger = l.Sugar()
	if err != nil {
		panic(err)
	}
	defer Logger.Sync()
	return Logger
}

// versionCMD prints version information
func versionCMD(c *cli.Context) error {
	fmt.Printf("Version: %s\n", version)
	fmt.Printf("Git Commit Hash: %s\n", commit)
	fmt.Printf("Build Date: %s\n", date)
	return nil
}

// stop - stops the agent
func stop(c *cli.Context) error {
	certs, err := GetCerts()
	if err != nil {
		return fmt.Errorf("Failed to load certificates: %s", err)
	}
	_, _, err = LoadConf(certs)
	if err != nil {
		return err
	}
	pidf, err := pidfile.Open(PIDFilePath())
	if os.IsNotExist(err) {
		return ErrAgentNotRunning
	}
	if err != nil {
		return err
	}
	if !pidf.Running() {
		return ErrAgentNotRunning
	}
	pid, err := pidf.Read()
	if err != nil {
		return fmt.Errorf("Failed to read the pidfile: %s", err)
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("Failed to find the agetnt's process: %s", err)
	}
	fmt.Printf("Sending a SIGINT to agent process %d\n", pid)
	err = process.Signal(syscall.SIGINT)
	return err
}

// createPIDFile creates the pid file or returns an error if it exists
func createPIDFile() error {
	_, err := pidfile.New(PIDFilePath())
	if err == pidfile.ErrProcessRunning {
		return fmt.Errorf("agent is already running, doing nothing")
	}
	if err != nil {
		return fmt.Errorf("pid file creation failed: %q", err)
	}
	return nil
}

func forkAgent(address httpserver.AddressType) (int, error) {
	pidf, err := pidfile.Open(PIDFilePath())
	if pidf != nil && !os.IsNotExist(err) && pidf.Running() {
		fmt.Println("agent is already running, doing nothing")
		return 0, nil
	}
	// start the agent process and exit
	execPath, err := osext.Executable()
	if err != nil {
		return 0, fmt.Errorf("Failed to find the executable: %s", err)
	}
	cmd := exec.Command("bash", "-c",
		fmt.Sprintf("%s start --agent --address %s >> %s",
			execPath, string(address), Conf.logFilePath))
	cmd.Env = nil
	err = cmd.Start()
	if err != nil {
		return 0, fmt.Errorf("agent failed to start :%q", err)
	}
	time.Sleep(100 * time.Millisecond)
	return cmd.Process.Pid, nil
}

// start - start the user's agent
func start(c *cli.Context) error {
	// test if the config directory exists

	homePath := ConfPath("")
	fmt.Printf("Home path: %s\n", homePath)
	_, err := os.Stat(homePath)
	if os.IsNotExist(err) {
		fmt.Printf("%s does not exist, initializing\n", homePath)
		initCMD(c)
	}
	certs, err := GetCerts()
	if err != nil {
		return fmt.Errorf("Failed to get the certificates: %s", err)
	}
	_, address, err := LoadConf(certs)
	if err != nil {
		return err
	}
	if c.IsSet("address") {
		address = httpserver.AddressType(c.String("address"))
	}
	// TODO: do we need this?
	peers.PtyMux = peers.PtyMuxType{}
	debug := c.Bool("debug")
	var loggerOption fx.Option
	if debug {
		loggerOption = fx.Provide(InitDevLogger)
		err := createPIDFile()
		if err != nil {
			return err
		}
	} else {
		if !c.Bool("agent") {
			pid, err := forkAgent(address)
			if err != nil {
				return err
			}
			fmt.Printf("agent started as process #%d\n", pid)
			versionNote := getVersionNote()
			if versionNote != "" {
				fmt.Println(versionNote)
			}

			return nil
		} else {
			loggerOption = fx.Provide(InitAgentLogger)
			err := createPIDFile()
			if err != nil {
				return err
			}
		}
	}
	// the code below runs for both --debug and --agent
	sigChan := make(chan os.Signal, 1)
	app := fx.New(
		loggerOption,
		fx.Supply(""),
		fx.Provide(
			LoadConf,
			httpserver.NewConnectHandler,
			fx.Annotate(NewFileAuth, fx.As(new(httpserver.AuthBackend))),
			NewSockServer,
			NewPeerbookClient,
			GetCerts,
			func() SocketStartParams {
				return SocketStartParams{RunPath("webexec.sock")}
			},
		),
		fx.Invoke(httpserver.StartHTTPServer, StartSocketServer, StartPeerbookClient),
	)
	if debug {
		app.Run()
		return nil
	} else {
		err = app.Start(context.Background())
	}
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	Logger.Infof("Shutting down")
	os.Remove(PIDFilePath())
	return nil
}

// restart function restarts the agent or starts it if it is stopped
func restart(c *cli.Context) error {
	err := stop(c)
	if err != nil && err != ErrAgentNotRunning {
		return err
	}
	// wait for the process to stop
	// TODO: https://github.com/tuzig/webexec/issues/18
	time.Sleep(1 * time.Second)
	return start(c)
}

// newSocketClient creates a new http client for the unix socket
func newSocketClient() http.Client {
	return http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", GetSockFP())
			},
		},
	}
}

// accept function accepts offers to connect
func accept(c *cli.Context) error {
	certs, err := GetCerts()
	if err != nil {
		return err
	}
	_, _, err = LoadConf(certs)
	if err != nil {
		return err
	}

	pid, err := getAgentPid()
	if err != nil {
		return err
	}
	if pid == 0 {
		// start the agent
		var address httpserver.AddressType
		if c.IsSet("address") {
			address = httpserver.AddressType(c.String("address"))
		} else {
			address = defaultHTTPServer
		}
		_, err = forkAgent(address)
		if err != nil {
			return fmt.Errorf("Failed to fork agent: %s", err)
		}
	}
	httpc := newSocketClient()
	// First get the agent's status and print to the clist
	var msg string
	var r *http.Response
	for i := 0; i < 30; i++ {
		r, err = httpc.Get("http://unix/status")
		if err == nil {
			goto gotstatus
		}
		time.Sleep(100 * time.Millisecond)
	}
	msg = "Failed to communicate with agent"
	fmt.Println(msg)
	return fmt.Errorf(msg)
gotstatus:
	defer r.Body.Close()
	body, _ := ioutil.ReadAll(r.Body)
	if r.StatusCode == http.StatusNoContent {
		return fmt.Errorf("didn't get a status from the agent")
	} else if r.StatusCode != http.StatusOK {
		return fmt.Errorf("agent's socket GET status return: %d", r.StatusCode)
	}
	fmt.Println(string(body))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go TrickleCandidates(ctx, httpc)
	// wait for signal interrupt or SIGTERM
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	return nil
}
func TrickleCandidates(ctx context.Context, httpc http.Client) {
	id := ""
	can := []byte{}
	for {
		select {
		case <-ctx.Done():
			return
		default:
			line, err := terminal.ReadPassword(0)
			if err != nil {
				fmt.Printf("ReadPassword error: %s", err)
				os.Exit(1)
			}
			can = append(can, line...)
			var js json.RawMessage
			// If it's not the end of a candidate, continue reading
			if len(line) == 0 || line[len(line)-1] != '}' || json.Unmarshal(can, &js) != nil {
				continue
			}
			if id == "" {
				resp, err := httpc.Post(
					"http://unix/offer/", "application/json", bytes.NewBuffer(can))
				if err != nil {
					fmt.Fprintf(os.Stderr, "Failed to POST agent's unix socket: %s", err)
					os.Exit(1)
				}
				if resp.StatusCode != http.StatusOK {
					msg, _ := ioutil.ReadAll(resp.Body)
					defer resp.Body.Close()
					fmt.Fprintf(os.Stderr, "Agent returned an error: %s", msg)
					os.Exit(1)
				}
				var body map[string]string
				json.NewDecoder(resp.Body).Decode(&body)
				defer resp.Body.Close()
				id = body["id"]
				delete(body, "id")
				msg, err := json.Marshal(body)
				fmt.Println(string(msg))
				go getAgentCandidates(ctx, httpc, id)
			} else {
				req, err := http.NewRequest("PUT", "http://unix/offer/"+id, bytes.NewReader(can))
				if err != nil {
					fmt.Fprintf(os.Stderr, "Failed to create new PUT request: %q", err)
				}
				req.Header.Set("Content-Type", "application/json")
				resp, err := httpc.Do(req)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Failed to PUT candidate: %q", err)
					os.Exit(1)
				}
				if resp.StatusCode != http.StatusOK {
					// msg, _ := ioutil.ReadAll(resp.Body)
					// defer resp.Body.Close()
					// print error on stderr
					fmt.Fprintf(os.Stderr, "Got a server error when PUTing: %v", resp.StatusCode)
				}
			}
		}
	}
	can = []byte{}
}
func getAgentCandidates(ctx context.Context, httpc http.Client, id string) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			can, err := getCandidate(httpc, id)
			if err != nil {
				return
			}
			if len(can) > 0 {
				fmt.Println(can)
			} else {
				os.Exit(0)
			}
		}
	}
}
func getCandidate(httpc http.Client, id string) (string, error) {
	r, err := httpc.Get("http://unix/offer/" + id)
	if err != nil {
		return "", fmt.Errorf("Failed to get candidate from the unix socket: %s", err)
	}
	body, _ := ioutil.ReadAll(r.Body)
	defer r.Body.Close()
	if r.StatusCode == http.StatusNoContent {
		return "", nil
	} else if r.StatusCode != http.StatusOK {
		return "", fmt.Errorf("agent's socker return status: %d", r.StatusCode)
	}
	return string(body), nil
}

// status function prints the status of the agent
func getAgentPid() (int, error) {
	fp := PIDFilePath()
	pidf, err := pidfile.Open(fp)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if !pidf.Running() {
		os.Remove(fp)
		return 0, nil
	}
	return pidf.Read()
}
func statusCMD(c *cli.Context) error {
	pid, err := getAgentPid()
	if err != nil {
		return err
	}
	if pid == 0 {
		fmt.Println("Agent is not running")
	} else {
		fmt.Printf("Agent is running with process id %d\n", pid)
	}
	// TODO: Get the the fingerprints of connected peers from the agent using the status socket
	fp := getFP()
	if fp == "" {
		fmt.Println("Unitialized, please run `webexec init`")
	} else {
		fmt.Printf("Fingerprint:  %s\n", fp)
	}
	httpc := newSocketClient()
	resp, err := httpc.Get("http://unix/status")
	if err != nil {
		return fmt.Errorf("Failed to get the agent's status: %s", err)
	}
	var pairs []CandidatePairValues
	// read the response into pair usin json.NewDecoder
	err = json.NewDecoder(resp.Body).Decode(&pairs)
	if err != nil {
		if err == io.EOF {
			fmt.Println("No connected peers")
			return nil
		}
		return fmt.Errorf("Failed to decode the agent's status: %s", err)
	}
	fmt.Println("\nConnected peers:")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', 0)
	writeICEPairsHeader(w)
	for _, pair := range pairs {
		pair.Write(w)
	}
	w.Flush()
	return nil
}
func initCMD(c *cli.Context) error {
	// init the dev logger so log messages are printed on the console
	InitDevLogger()
	homePath := ConfPath("")
	_, err := os.Stat(homePath)
	if os.IsNotExist(err) {
		err = os.MkdirAll(homePath, 0755)
		if err != nil {
			return err
		}
		fmt.Printf("Created %q directory\n", homePath)
	} else {
		return fmt.Errorf("%q already exists, leaving as is.", homePath)
	}
	fPath := ConfPath("certnkey.pem")
	key = &KeyType{Name: fPath}
	cert, err := key.generate()
	if err != nil {
		return cli.Exit(fmt.Sprintf("Failed to create certificate: %s", err), 2)
	}
	key.save(cert)
	// TODO: add a CLI option to make it !sillent
	fmt.Printf("Created certificate in: %s\n", fPath)
	uid := os.Getenv("PEERBOOK_UID")
	pbHost := os.Getenv("PEERBOOK_HOST")
	name := os.Getenv("PEERBOOK_NAME")
	if name == "" {
		dn, err := os.Hostname()
		if err != nil {
			return fmt.Errorf("Failed to get hostname: %s", err)
		}
		// let the user edit the host name
		fmt.Printf("Enter a name for this host [%s]: ", dn)
		reader := bufio.NewReader(os.Stdin)
		text, _ := reader.ReadString('\n')
		text = strings.TrimSpace(text)
		if text != "" {
			name = text
		} else {
			name = dn
		}
	}
	confPath, err := createConf(uid, pbHost, name)
	fmt.Printf("Created dotfile in: %s\n", confPath)
	if err != nil {
		return err
	}
	_, _, err = LoadConf([]webrtc.Certificate{*cert})
	if err != nil {
		return cli.Exit(fmt.Sprintf("Failed to parse default conf: %s", err), 1)
	}
	fp := getFP()
	fmt.Printf("Fingerprint:  %s\n", fp)
	if uid != "" {
		verified, err := verifyPeer(Conf.peerbookHost)
		if err != nil {
			return fmt.Errorf("Got an error verifying peer: %s", err)
		}
		if verified {
			fmt.Println("Verified by peerbook")
		} else {
			fmt.Println("Unverified by peerbook. Please use terminal7 to verify the fingerprint")
		}
	}
	return nil
}
func upgrade(c *cli.Context) error {
	if getVersionNote() == "" {
		fmt.Println("You are already running the latest version")
		return nil
	}
	resp, err := http.Get("https://get.webexec.sh")
	if err != nil {
		return fmt.Errorf("Failed to get upgrade script: %s", err)
	}
	defer resp.Body.Close()
	script, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("Failed to read upgrade script: %s", err)
	}

	cmd := exec.Command("bash", "-c", string(script))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// copyCMD copies data from stdin to the clipboard
func copyCMD(c *cli.Context) error {
	b, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("Failed to read from stdin: %s", err)
	}
	mimeType := http.DetectContentType(b)
	fmt.Fprintf(os.Stderr, "copying mimetype: %s\n", mimeType) // Outputs the MIME type, e.g., text/plain

	fp := GetSockFP()
	_, err = os.Stat(fp)
	if os.IsNotExist(err) {
		return fmt.Errorf("Agent is not running. Please run `webexec start`")
	}
	httpc := http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", fp)
			},
		},
	}
	resp, err := httpc.Post("http://unix/clipboard", mimeType, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("Failed to create the request: %s", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Failed to send the copy request: %s", resp.Status)
	}
	return nil
}
func pasteCMD(c *cli.Context) error {
	fp := GetSockFP()
	_, err := os.Stat(fp)
	if os.IsNotExist(err) {
		fmt.Println("Agent is not running. Please run `webexec start`")
	}
	httpc := http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", fp)
			},
		},
	}
	resp, err := httpc.Get("http://unix/clipboard")
	if err != nil {
		return fmt.Errorf("Failed to communicate with agent: %s", err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("Failed to read clipboard content: %s", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Failed to get clipboard content: %s: %s", resp.Status, body)
	}
	fmt.Print(string(body))
	return nil
}

// handleCTRLMsg handles incoming control messages
func handleCTRLMsg(peer *peers.Peer, m peers.CTRLMessage, raw json.RawMessage) {
	switch m.Type {
	case "resize":
		handleResize(peer, m, raw)
	case "restore":
		handleRestore(peer, m, raw)
	case "get_payload":
		handleGetPayload(peer, m)
	case "set_payload":
		handleSetPayload(peer, m, raw)
	case "mark":
		handleMark(peer, m)
	case "reconnect_pane":
		handleReconnectPane(peer, m, raw)
	case "add_pane":
		handleAddPane(peer, m, raw)
	default:
		Logger.Errorf("Got a control message with unknown type: %q", m.Type)
		// send nack
		err := peer.SendNack(m, "unknown control message type")
		if err != nil {
			Logger.Errorf("#%d: Failed to send nack: %v", peer.FP, err)
		}
	}
	return
}

func main() {
	app := &cli.App{
		Name:        "webexec",
		Usage:       "execute commands and pipe their stdin&stdout over webrtc",
		HideVersion: true,
		Commands: []*cli.Command{
			{
				Name:        "client",
				Usage:       "manage clients",
				Subcommands: ClientCommands,
			}, {
				Name:   "version",
				Usage:  "Print version information",
				Action: versionCMD,
			}, {
				Name:  "restart",
				Usage: "restarts the agent",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "address",
						Aliases: []string{"a"},
						Usage:   "The address to listen to",
						Value:   "0.0.0.0:7777",
					},
				},
				Action: restart,
			}, {
				Name:    "start",
				Aliases: []string{"l"},
				Usage:   "Spawns a backgroung http server & webrtc peer",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "address",
						Aliases: []string{"a"},
						Usage:   "The address to listen to",
						Value:   "",
					},
					&cli.BoolFlag{
						Name:  "debug",
						Usage: "Run in debug mode in the foreground",
					},
					&cli.BoolFlag{
						Name:  "agent",
						Usage: "Run as agent, in the background",
					},
				},
				Action: start,
			}, {
				Name:   "status",
				Usage:  "webexec agent's status",
				Action: statusCMD,
			}, {
				Name:   "stop",
				Usage:  "stop the user's agent",
				Action: stop,
			}, {
				Name:   "init",
				Usage:  "initialize the conf file",
				Action: initCMD,
			}, {
				Name:   "accept",
				Usage:  "accepts an offer to connect",
				Action: accept,
			}, {
				Name:   "upgrade",
				Usage:  "upgrades webexec to the latest version",
				Action: upgrade,
			},
			{
				Name:   "copy",
				Usage:  "Copy data from stdin to the active peer's clipboard. If no active peer, use local clipboard",
				Action: copyCMD,
			}, {
				Name:   "paste",
				Usage:  "Paste data from the active peer's clipboard to stdout. If no active peer, use local clipboard",
				Action: pasteCMD,
			},
		},
	}
	err := app.Run(os.Args)
	if err != nil {
		fmt.Println(err)
	}
}
