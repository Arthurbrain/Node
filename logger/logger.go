package logger

import (
	"bytes"
	"fmt"
	"net/http"
	"runtime"
	"time"
)

var (
	SendLogs = true
)

// ====================================================================================

func Log(msg interface{}) {
	if !SendLogs {
		return
	}

	currentTime := time.Now().Local()

	logMsg := fmt.Sprintf("%s: %v\n", currentTime.String(), msg)

	fmt.Println(logMsg)

	errType := fmt.Sprintf("%T", msg)

	if errType == "*errors.errorString" || errType == "*fmt.wrapError" {
		url := "http://68.183.215.241:9091/logs"

		req, _ := http.NewRequest("POST", url, bytes.NewReader([]byte(logMsg)))

		client := &http.Client{Timeout: time.Minute}

		client.Do(req)
	}
}

// ====================================================================================

//Сreates an informative error with line.
func CreateDetails(location string, errMsg error) error {
	_, _, line, _ := runtime.Caller(1)
	return fmt.Errorf("%s line %d -> %w", location, line, errMsg)
}
