// Author: @ysqi

package utils

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"gitee.com/nerthus/nerthus/common/math"
)

func TestPathExpansion(t *testing.T) {
	home := HomeDir()
	tests := map[string]string{
		"/home/someuser/tmp":  "/home/someuser/tmp",
		"~/tmp":               filepath.Join(home, "tmp"),
		"~\\tmp":              filepath.Join(home, "tmp"),
		"./tmp":               "tmp",
		"~thisOtherUser/b/":   "~thisOtherUser/b",
		"$DDDXXX/a/$DDDXXX/b": "/tmp/a/tmp/b",
		"/a/b/":               "/a/b",
		"":                    "",
	}

	os.Setenv("DDDXXX", "/tmp")
	for test, expected := range tests {
		got := expandPath(test)
		if got != expected {
			t.Errorf("expand path %q, got %s, expected %s\n", test, got, expected)
		}
	}
}

func TestFilepathString(t *testing.T) {
	var p FilepathString
	if err := p.Set("./test/test/a/b/c"); err == nil {
		t.Fatal("want get error,but got nil")
	}

	dir, err := ioutil.TempDir("", "nerthustest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	err = p.Set(dir)
	if err != nil {
		t.Fatalf("want set success,but got error:%s", err.Error())
	}
	if p.String() != dir {
		t.Fatalf("want get %s,but got %s", dir, p.String())
	}

	file, err := os.Create(filepath.Join(dir, "test.file"))
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	err = p.Set(file.Name())
	if err != nil {
		t.Fatalf("want set success,but got error:%s", err.Error())
	}
	if p.String() != file.Name() {
		t.Fatalf("want get %s,but got %s", dir, p.String())
	}
}

func TestBigValue(t *testing.T) {
	name := strconv.FormatInt(int64(time.Now().Nanosecond()), 10)
	flag := newBigIntFlag(name, nil, "")

	value := "0x18F8F8F1000111000110011100222004330052300000000000000000FEFCF3CC"
	want := math.MustParseBig256("0x18F8F8F1000111000110011100222004330052300000000000000000FEFCF3CC")

	err := defaultFlags.Set(name, value)
	if err != nil {
		t.Fatal(err)
	}
	if flag.Value.String() != want.String() {
		t.Fatalf("want get bit int %s,bug got %s", want.String(), flag.Value.String())
	}

	got := defaultFlags.GetBigInt(name)
	if got.Cmp(want) != 0 {
		t.Fatalf("want get bit int %v,bug got %v", want, got)
	}

	// Empty is zero
	err = defaultFlags.Set(name, "")
	if err != nil {
		t.Fatal(err)
	}
	if flag.Value.String() != "0" {
		t.Fatalf("want get bit int 0,bug got %s", flag.Value.String())
	}
}
