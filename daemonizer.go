package daemonizer

import (
	"encoding/json"
	"errors"
	"io"
	"os"
)

const (
	DaemonProcessArgumentName = "daemon_process"
	DaemonProcessArgument     = "--" + DaemonProcessArgumentName
)

var (
	// ErrNotParentProcess indicates that the process is not a parent process.
	ErrNotParentProcess = errors.New("not parent process")
	// ErrNotDaemonProcess indicates that the process is not a daemon process.
	ErrNotDaemonProcess = errors.New("not daemon process")
	// ErrRunDaemonFailed indicates that the daemon process failed to run.
	ErrRunDaemonFailed = errors.New("failed to run daemon process")
	// ErrParamNotSerializable indicates that the param is not JSON serializable.
	ErrParamNotSerializable = errors.New("param is not JSON serializable")
	// ErrParamSendFailed indicates that the param failed to send to the daemon process.
	ErrParamSendFailed = errors.New("failed to send param to daemon process")
)

type daemonProcessInitRequest struct {
	Params map[string]interface{} `json:"params"`
}

type daemonProcessResponse struct {
	Status  string `json:"status"` // "success" or "error"
	Message string `json:"message"`
}

type Daemonizer struct {
	daemon           bool
	commandArguments []string
	params           map[string]interface{} // must be serializable

	daemonProc      *os.Process
	paramReadPipe   *os.File
	paramWritePipe  *os.File
	outputReadPipe  *os.File
	outputWritePipe *os.File
}

type DaemonizeOption struct {
	Dir    string   // if empty, inherit from parent process
	Stdin  *os.File // if nil, inherit from parent process
	Stdout *os.File // if nil, inherit from parent process
	Stderr *os.File // if nil, inherit from parent process
	Env    []string // if nil, inherit from parent process
}

func (do *DaemonizeOption) InheritParentIO() {
	do.Dir = ""
	do.Env = nil

	do.Stdin = os.Stdin
	do.Stdout = os.Stdout
	do.Stderr = os.Stderr
}

func (do *DaemonizeOption) UseNullIO() error {
	stdin, err := os.Open(os.DevNull)
	if err != nil {
		return err
	}

	stdout, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return err
	}

	stderr, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return err
	}

	do.Stdin = stdin
	do.Stdout = stdout
	do.Stderr = stderr
	return nil
}

// NewDaemonizer creates a new Daemonizer instance.
func NewDaemonizer() (*Daemonizer, error) {
	// check if the process is a d
	d := &Daemonizer{}
	d.readCommandArguments()

	if d.daemon {
		err := d.initDaemonProcess()
		if err != nil {
			return nil, err
		}
	}

	return d, nil
}

func (d *Daemonizer) readCommandArguments() {
	cleanCommandArguments := []string{}
	for _, arg := range os.Args {
		if arg == DaemonProcessArgument {
			d.daemon = true
		} else {
			cleanCommandArguments = append(cleanCommandArguments, arg)
		}
	}

	d.commandArguments = cleanCommandArguments
	if d.daemon {
		// overwrite Args
		os.Args = cleanCommandArguments
	}
}

func (d *Daemonizer) IsDaemon() bool {
	return d.daemon
}

// Daemonize starts the daemon process with the given params.
// called by the parent process
func (d *Daemonizer) Daemonize(params map[string]interface{}, option DaemonizeOption) error {
	if d.daemon {
		return ErrNotParentProcess
	}

	// set params
	d.params = params

	err := d.runDaemonProcess(&option)
	if err != nil {
		return err
	}

	err = d.sendParamsToDaemonProcess()
	if err != nil {
		d.outputReadPipe.Close()
		return err
	}

	// read output from the daemon process
	daemonProcessResponse := d.readOutputFromDaemonProcess()
	for msg := range daemonProcessResponse {
		switch msg.Status {
		case "success":
			return nil
		case "error":
			return errors.New(msg.Message)
		}
	}

	return nil
}

func (d *Daemonizer) GetParams() map[string]interface{} {
	return d.params
}

func (d *Daemonizer) GetCommandArguments() []string {
	return d.commandArguments
}

// runDaemonProcess starts the daemon process.
// called by the parent process
func (d *Daemonizer) runDaemonProcess(option *DaemonizeOption) error {
	// create pipes
	paramR, paramW, err := os.Pipe()
	if err != nil {
		return err
	}

	outputR, outputW, err := os.Pipe()
	if err != nil {
		return err
	}

	attr := &os.ProcAttr{}

	attr.Files = []*os.File{os.Stdin, os.Stdout, os.Stderr, paramR, outputW}

	if option != nil {
		if option.Dir != "" {
			attr.Dir = option.Dir
		}
		if option.Stdin != nil {
			attr.Files[0] = option.Stdin
		}
		if option.Stdout != nil {
			attr.Files[1] = option.Stdout
		}
		if option.Stderr != nil {
			attr.Files[2] = option.Stderr
		}
		if option.Env != nil {
			attr.Env = option.Env
		}
	}

	// start the child process
	newCommandArguments := []string{}
	for argIdx, arg := range d.commandArguments {
		newCommandArguments = append(newCommandArguments, arg)
		if argIdx == 0 {
			newCommandArguments = append(newCommandArguments, DaemonProcessArgument)
		}
	}

	proc, err := os.StartProcess(d.commandArguments[0], newCommandArguments, attr)
	if err != nil {
		return err
	}

	d.paramReadPipe = paramR
	d.paramWritePipe = paramW
	d.outputReadPipe = outputR
	d.outputWritePipe = outputW
	d.daemonProc = proc

	return nil
}

// sendParamsToDaemonProcess sends the params to the daemon process.
// called by the parent process
func (d *Daemonizer) sendParamsToDaemonProcess() error {
	defer d.paramWritePipe.Close()

	daemonProcessInitRequest := &daemonProcessInitRequest{
		Params: d.params,
	}

	paramBytes, err := json.Marshal(daemonProcessInitRequest)
	if err != nil {
		return ErrParamNotSerializable
	}

	_, err = d.paramWritePipe.Write(paramBytes)
	if err != nil {
		return ErrParamSendFailed
	}

	return nil
}

// readOutputFromDaemonProcess reads the output from the daemon process.
// called by the parent process
func (d *Daemonizer) readOutputFromDaemonProcess() chan daemonProcessResponse {
	responseChan := make(chan daemonProcessResponse)

	go func() {
		defer d.outputReadPipe.Close()

		decoder := json.NewDecoder(d.outputReadPipe)

		for {
			var msg daemonProcessResponse
			err := decoder.Decode(&msg)

			if err == io.EOF {
				break
			}
			if err != nil {
				responseChan <- daemonProcessResponse{
					Status:  "error",
					Message: err.Error(),
				}
				break
			}

			responseChan <- msg
		}

		close(responseChan)
	}()

	return responseChan
}

// initDaemonProcess initializes the daemon process.
// called by the daemon process
func (d *Daemonizer) initDaemonProcess() error {
	if !d.daemon {
		return ErrNotDaemonProcess
	}

	// init pipes
	d.paramReadPipe = os.NewFile(uintptr(3), "paramReadPipe")     // fd 3
	d.outputWritePipe = os.NewFile(uintptr(4), "outputWritePipe") // fd 4

	err := d.readParamsFromParentProcess()
	if err != nil {
		// write error to the parent process
		d.sendResponseToParentProcess("error", err.Error())
		return err
	}

	// write success to the parent process
	err = d.sendResponseToParentProcess("success", "daemon process started successfully")
	if err != nil {
		return err
	}
	return nil
}

// readParamsFromParentProcess reads the params from the parent process.
// called by the daemon process
func (d *Daemonizer) readParamsFromParentProcess() error {
	defer d.paramReadPipe.Close()

	decoder := json.NewDecoder(d.paramReadPipe)

	var msg daemonProcessInitRequest
	err := decoder.Decode(&msg)

	if err != nil {
		return err
	}

	d.params = msg.Params
	return nil
}

// sendResponseToParentProcess sends the response to the parent process.
// called by the daemon process
func (d *Daemonizer) sendResponseToParentProcess(status string, message string) error {
	defer d.outputWritePipe.Close()

	response := &daemonProcessResponse{
		Status:  status,
		Message: message,
	}

	encoder := json.NewEncoder(d.outputWritePipe)
	err := encoder.Encode(response)
	if err != nil {
		return err
	}

	return nil
}
