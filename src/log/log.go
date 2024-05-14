// Copyright 2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may not
// use this file except in compliance with the License. A copy of the
// License is located at
//
// http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
// either express or implied. See the License for the specific language governing
// permissions and limitations under the License.

// Package log is used to initialize the logger.
package log

import (
	"fmt"
	logging "log"
	"os"
)

const LOG_LEVEL = "WARN"

var LogLevels = map[string]int{
	"TRACE":  1,
	"DEBUG":  2,
	"INFO":   3,
	"WARN":   4,
	"ERROR":  5,
	"ALWAYS": 5,
}
var Log logging.Logger

func init() {
	Log = *logging.New(os.Stdout, "INFO: ", logging.Ldate|logging.Ltime)
}

func displayMessage(level, msg string) {
	if LogLevels[level] >= LogLevels[LOG_LEVEL] {
		Log.SetPrefix(fmt.Sprintf("%s: ", level))
		Log.Println(msg)
	}
}

func Trace(msg string) {
	displayMessage("TRACE", msg)
}

func Tracef(msg string, v ...any) {
	Trace(fmt.Sprintf(msg, v...))
}

func Debug(msg string) {
	displayMessage("DEBUG", msg)
}

func Debugf(msg string, v ...any) {
	Debug(fmt.Sprintf(msg, v...))
}

func Info(msg string) {
	displayMessage("INFO", msg)
}

func Infof(msg string, v ...any) {
	Info(fmt.Sprintf(msg, v...))
}

func Warn(msg string) {
	displayMessage("WARN", msg)
}

func Warnf(msg string, v ...any) {
	Warn(fmt.Sprintf(msg, v...))
}

func Error(msg string) {
	displayMessage("ERROR", msg)
}

func Errorf(msg string, v ...any) {
	Error(fmt.Sprintf(msg, v...))
}

func Always(msg string) {
	displayMessage("ALWAYS", msg)
}

func Alwaysf(msg string, v ...any) {
	Always(fmt.Sprintf(msg, v...))
}
