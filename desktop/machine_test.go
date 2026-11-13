package desktop

import (
	"reflect"
	"strings"
	"testing"
)

// machineWordsIn lists the fields that still name a Mac.
func machineWordsIn(t text) []string {
	var found []string
	v := reflect.ValueOf(t)
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		if field.Kind() != reflect.String {
			continue
		}
		if strings.Contains(field.String(), "Mac") {
			found = append(found, v.Type().Field(i).Name)
		}
	}
	return found
}

// TestFitMachineNamesTheComputer keeps a Windows window from saying Mac, and
// leaves the macOS wording alone.
func TestFitMachineNamesTheComputer(t *testing.T) {
	for _, l := range []lang{langEnglish, langChinese} {
		base := texts(l)
		if base.ThisMac == "" {
			t.Fatal("the catalog holds no wording for this computer")
		}

		if got := fitMachine(base, "darwin"); got.ThisMac != base.ThisMac {
			t.Fatalf("%v: macOS wording changed to %q", l, got.ThisMac)
		}

		other := fitMachine(base, "windows")
		if fields := machineWordsIn(other); len(fields) > 0 {
			t.Fatalf("%v: the Windows wording still says Mac in %v", l, fields)
		}
		if other.CheckingThisMac == base.CheckingThisMac {
			t.Fatalf("%v: the Windows wording kept the macOS text", l)
		}
		if other.ThisMac == "" {
			t.Fatalf("%v: the Windows wording names no computer", l)
		}
	}
}
