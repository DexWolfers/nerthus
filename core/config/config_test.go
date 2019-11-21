package config

import (
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestDefaultDataDir(t *testing.T) {

	t.Run("env", func(t *testing.T) {
		caces := []string{
			"./a/b/c",
			"/o/p",
		}

		for _, c := range caces {
			os.Setenv(datadirPathEnv, c)
			got := DefaultDataDir()
			require.Equal(t, c, got)
		}
	})

}

func TestConfig_dirExists(t *testing.T) {
	Convey("dirExists", t, func() {

		tmpF := os.TempDir()
		exists := dirExists(tmpF)
		So(exists, ShouldBeTrue)
		assert.True(t, exists,
			"%s directory exists but dirExists returned false", tmpF)

		tmpF = "/blah-" + time.Now().Format(time.RFC3339Nano)
		exists = dirExists(tmpF)
		So(exists, ShouldBeFalse)
		assert.False(t, exists,
			"%s directory does not exist but dirExists returned true",
			tmpF)

	})
}

func TestConfig_AddDevConfigPath(t *testing.T) {
	Convey("AddDevConfigPath", t, func() {

		// Case 1: use viper instance to call AddDevConfigPath
		v := viper.New()
		err := AddDevConfigPath(v)
		So(err, ShouldBeNil)

		// Case 2: default viper instance to call AddDevConfigPath
		err = AddDevConfigPath(nil)
		So(err, ShouldBeNil)

		// Error case: GOPATH is empty
		gopath := os.Getenv("GOPATH")
		os.Setenv("GOPATH", "")
		defer os.Setenv("GOPATH", gopath)
		err = AddDevConfigPath(v)
		So(err, ShouldEqual, ErrMissingGoPath)
	})
}

func TestConfig_InitViper(t *testing.T) {
	Convey("InitViper", t, func() {
		// Case 1: use viper instance to call InitViper
		v := viper.New()
		err := InitViper(v, "")
		So(err, ShouldBeNil)

		// Case 2: default viper instance to call InitViper
		err = InitViper(nil, "")
		So(err, ShouldBeNil)
	})
}

func TestConfig_GetPath(t *testing.T) {
	Convey("GetPath", t, func() {
		// Case 1: non existent viper property
		path := GetPath("foo")
		So(path, ShouldBeEmpty)
		// Case 2: viper property that has absolute path
		viper.Set("testpath", "/test/config.yml")
		path = GetPath("testpath")
		So(path, ShouldEqual, "/test/config.yml")
	})
}

func TestConfig_TranslatePathInPlace(t *testing.T) {
	Convey("TranslatePathInPlace", t, func() {
		// Case 1: relative path
		p := "foo"
		TranslatePathInPlace(OfficialPath, &p)
		So(p, ShouldNotEqual, "foo")

		// Case 2: absolute path
		p = "/foo"
		TranslatePathInPlace(OfficialPath, &p)
		So(p, ShouldEqual, "/foo")
	})
}

func TestConfig_GetDevMspDir(t *testing.T) {
	Convey("GetDevMspDir", t, func() {
		// Success case
		_, err := GetDevMspDir()
		So(err, ShouldBeNil)

		// Error case: GOPATH is empty
		gopath := os.Getenv("GOPATH")
		os.Setenv("GOPATH", "")
		defer os.Setenv("GOPATH", gopath)
		_, err = GetDevMspDir()
		So(err, ShouldNotBeNil)

		// Error case: GOPATH is set to temp dir
		dir, err1 := ioutil.TempDir("/tmp", "devmspdir")
		So(err1, ShouldBeNil)
		defer os.RemoveAll(dir)
		os.Setenv("GOPATH", dir)
		_, err = GetDevMspDir()
		So(err, ShouldNotBeNil)
	})
}

func TestConfig_GetDevConfigDir(t *testing.T) {
	Convey("GetDevConfigDir", t, func() {
		gopath := os.Getenv("GOPATH")
		os.Setenv("GOPATH", "")
		defer os.Setenv("GOPATH", gopath)
		_, err := GetDevConfigDir()
		So(err, ShouldNotBeNil)
	})
}

func TestMustInt(t *testing.T) {

	t.Run("default", func(t *testing.T) {
		got := MustInt("missing", 100)
		require.Equal(t, 100, got)
	})
	t.Run("real", func(t *testing.T) {
		viper.Set("myvalue.int", 120)
		got := MustInt("myvalue.int", 12)
		require.Equal(t, 120, got, "must equal set value")
	})
	t.Run("panic", func(t *testing.T) {
		viper.Set("myvalue.string", "abc")

		require.Panics(t, func() {
			MustInt("myvalue.string", 120)
		})
	})

	t.Run("canEmpty", func(t *testing.T) {
		viper.Set("myvalue.str", "")
		require.NotPanics(t, func() {
			got := MustInt("myvalue.str", 12)
			require.Equal(t, 12, got)
		})
	})
}

func TestMustFloat(t *testing.T) {

	t.Run("default", func(t *testing.T) {
		got := MustFloat("missing", 100)
		require.Equal(t, float32(100), got)
	})
	t.Run("real", func(t *testing.T) {
		viper.Set("myvalue.int", 120)
		got := MustFloat("myvalue.int", 12)
		require.Equal(t, float32(120), got, "must equal set value")
	})
	t.Run("panic", func(t *testing.T) {
		viper.Set("myvalue.string", "abc")

		require.Panics(t, func() {
			MustFloat("myvalue.string", 120)
		})
	})

	t.Run("canEmpty", func(t *testing.T) {
		viper.Set("myvalue.str", "")
		require.NotPanics(t, func() {
			got := MustFloat("myvalue.str", 12)
			require.Equal(t, float32(12), got)
		})
	})
}
