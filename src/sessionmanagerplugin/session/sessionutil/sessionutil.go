// Package sessionutil provides utility for sessions.
package sessionutil

func NewDisplayMode() DisplayMode {
	displayMode := DisplayMode{}
	displayMode.InitDisplayMode()
	return displayMode
}
