package desktop

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/oneo-me/nsgo/nsgotest"

	exoCore "exodrive/internal/core"
)

// TestEveryWordExists keeps a language complete: a field with no sentence is a
// blank line in the window, and the compiler cannot see it.
func TestEveryWordExists(t *testing.T) {
	for name, words := range map[string]text{"english": englishText, "chinese": chineseText} {
		value := reflect.ValueOf(words)
		kind := value.Type()
		for i := 0; i < value.NumField(); i++ {
			if strings.TrimSpace(value.Field(i).String()) == "" {
				t.Errorf("%s.%s is empty", name, kind.Field(i).Name)
			}
		}
	}
}

// TestChineseDiffersFromEnglish keeps a copy of the English file from passing as
// a translation.
func TestChineseDiffersFromEnglish(t *testing.T) {
	en := reflect.ValueOf(englishText)
	zh := reflect.ValueOf(chineseText)
	kind := en.Type()
	for i := 0; i < en.NumField(); i++ {
		if en.Field(i).String() == zh.Field(i).String() {
			t.Errorf("%s holds the English words in Chinese", kind.Field(i).Name)
		}
	}
}

func TestLanguageOfReadsTheTag(t *testing.T) {
	cases := map[string]lang{
		"":           langEnglish,
		"en":         langEnglish,
		"en-US":      langEnglish,
		"zh":         langChinese,
		"zh-Hans-CN": langChinese,
		"zh-Hant":    langChinese,
		"fr":         langEnglish,
	}
	for tag, want := range cases {
		if got := langFor(tag); got != want {
			t.Errorf("langFor(%q) is %q, want %q", tag, got, want)
		}
	}
}

// TestWindowWritesInChinese mounts the window with the Chinese language set and
// reads the words a person presses.
func TestWindowWritesInChinese(t *testing.T) {
	t.Setenv(LangEnvVar, "zh")
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))

	app := nsgotest.Start(t, NewMain(newFakeEngine(fakeCache())), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:Fake Cache") })
	app.WaitUntil(2*time.Second, func() bool {
		dump := app.Dump()
		return strings.Contains(dump, "这台 Mac") && strings.Contains(dump, "缓存")
	})

	app.Type("id:target", t.TempDir())
	app.Settle()
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("id:select-all") })
	if !strings.Contains(app.Dump(), "全选") {
		t.Fatalf("the Chinese window offers no Chinese select action:\n%s", app.Dump())
	}
	if got := badgeText(app, fakeCache().ID); got != "在本机" {
		t.Fatalf("the card reads %q, want 在本机", got)
	}
}
