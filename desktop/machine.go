package desktop

import (
	"reflect"
	"runtime"
	"strings"
)

// machineWords pairs the wording for this Mac with the wording for any other
// system. A window on Windows says PC in the same places.
var machineWords = [][2]string{
	{"this Mac", "this PC"},
	{"This Mac", "This PC"},
	{"这台 Mac", "这台电脑"},
}

// fitMachine replaces the name of the computer when the system is not macOS,
// so a Windows window never says Mac. Every other word stays as it is.
func fitMachine(t text, goos string) text {
	if goos == "darwin" {
		return t
	}
	v := reflect.ValueOf(&t).Elem()
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		if field.Kind() != reflect.String || !field.CanSet() {
			continue
		}
		s := field.String()
		for _, word := range machineWords {
			s = strings.ReplaceAll(s, word[0], word[1])
		}
		field.SetString(s)
	}
	return t
}

// platformName is the system the window runs on.
func platformName() string { return runtime.GOOS }
