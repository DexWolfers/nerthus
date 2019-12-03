// Code generated by go-bindata.
// sources:
// ../nerthus.sql
// DO NOT EDIT!

package ntsdb

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func bindataRead(data []byte, name string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("Read %q: %v", name, err)
	}

	var buf bytes.Buffer
	_, err = io.Copy(&buf, gz)
	clErr := gz.Close()

	if err != nil {
		return nil, fmt.Errorf("Read %q: %v", name, err)
	}
	if clErr != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

type asset struct {
	bytes []byte
	info  os.FileInfo
}

type bindataFileInfo struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
}

func (fi bindataFileInfo) Name() string {
	return fi.name
}
func (fi bindataFileInfo) Size() int64 {
	return fi.size
}
func (fi bindataFileInfo) Mode() os.FileMode {
	return fi.mode
}
func (fi bindataFileInfo) ModTime() time.Time {
	return fi.modTime
}
func (fi bindataFileInfo) IsDir() bool {
	return false
}
func (fi bindataFileInfo) Sys() interface{} {
	return nil
}

var _NerthusSql = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\xcc\x58\x4f\x6f\x1b\x45\x14\x3f\x8f\x3f\xc5\x3b\x3a\xd2\x1a\x35\x69\x54\x50\xb9\xd4\x09\x5b\xb0\x48\x9c\xca\xd9\xa0\xe6\xb4\x9a\xec\x8e\xed\x69\xd6\x33\xcb\xce\xac\x13\x73\x4b\x0f\xa8\x42\x42\x1c\x4a\xc5\x39\x07\x20\x17\x9a\xde\x10\x41\x88\x2f\xd3\x24\xfe\x18\x68\x66\xbd\xff\x77\xed\x4d\x89\x04\x39\xc5\xf3\xde\xcc\xbc\xdf\xef\xfd\xe6\xbd\x67\x77\x3a\xd0\x0d\xe5\x98\x07\xf0\x18\xbe\x39\x79\xf1\x64\xe3\xc1\xfa\xc7\x9d\xf5\x07\x9d\xf5\x87\xad\x56\xa7\x03\x37\x6f\xde\xdd\x7c\xff\xf6\xfa\xea\xf5\xfc\xf2\xb7\xeb\x1f\x7e\x06\xbd\x38\xff\xee\xe5\xed\xcb\x3f\xde\xff\x7d\x7e\x73\x76\x39\x3f\xbf\x68\x6d\x0f\xcc\xae\x65\x82\xd5\xdd\xda\x31\xa1\xf7\x14\xfa\x7b\x16\x98\xcf\x7b\xfb\xd6\x3e\x30\xee\x12\x68\xb7\x00\x40\xff\x4b\x5d\x48\xfe\xa6\x38\x70\xc6\x38\x68\xaf\x6f\x7c\xb2\xa6\xb7\xf4\x0f\x76\x76\x8c\xd4\x95\x0d\x39\x42\x47\x1e\x3f\xca\x18\x91\x4f\xd9\x08\x21\x24\xe9\x84\x08\x89\x27\x7e\x62\x83\xcf\xcc\xa7\xdd\x83\x1d\x0b\xb6\x0f\x06\x03\xb3\x6f\xd9\x56\x6f\xd7\xdc\xb7\xba\xbb\xcf\xd4\x2e\xfe\x21\xbb\x86\x94\xb9\x43\x4c\x3d\x81\x10\x65\xd2\x68\xa1\x67\x83\xde\x6e\x77\x70\x08\x5f\x9a\x87\x08\xb5\x23\x3c\x6b\xad\xb5\x4f\x5b\xcb\x18\x20\x6c\x6a\x2b\xa4\x74\xaa\x88\x40\x01\x3e\xa1\x2e\xe8\x60\xd8\xac\x88\x8e\xb0\x69\xe0\xf9\xca\x9a\xb3\xe8\x2b\x14\xef\xbf\x9e\xcd\x2f\xcf\xde\x5f\x5d\x5d\x5f\xbd\x5e\x76\xe5\x09\x95\x8c\x08\x61\x7b\xf4\x48\x73\x1f\x32\x2a\xed\x31\x16\xe3\x0c\xeb\x8f\x1e\x65\x49\x47\xd8\x75\x03\x22\x04\x20\x40\xb1\xc7\xe6\x66\xce\xe3\x05\xa7\xcc\x76\xb1\x24\x9a\x8d\x92\x25\x73\x47\xcd\x15\x0e\x66\x0e\xf1\xf4\x11\x10\xf1\xb9\x58\x49\xb7\x66\x76\x1a\x2d\x24\x24\x96\xa1\x50\x6c\x44\xee\x94\x49\x32\x0a\xb0\x87\x2a\xf2\x01\xa8\x9d\x1c\x63\x2c\xc0\xac\xc5\xbc\xdd\xfe\x78\x71\xf3\xea\xf7\x94\xbd\x57\x3f\xcd\xcf\x2f\xe6\x6f\xdf\x5d\xff\xf5\xa6\x19\x8d\x42\x96\x78\xac\x83\x09\xe0\x8c\x31\x1b\x91\x08\x27\x44\x48\x01\x62\x7e\xf3\xd2\xdf\xdc\x5c\x53\xc6\x2c\x0e\x80\x14\xc8\x0a\x00\x97\xbf\xdc\xfe\xf9\x6d\x53\x00\xb6\x4b\x24\xa6\xde\x1d\x70\xd4\x86\x9c\xf3\xaa\x8b\xdd\x80\x5c\x16\x96\x44\xa9\xb6\xe8\xb0\x72\x12\x28\x55\x05\xec\xfb\x89\xf1\xe1\x46\x89\x72\xca\xd2\x10\x0b\x56\xea\x9e\xc2\x56\xef\xf3\x5e\xdf\xca\x6f\x0a\x08\x96\x24\xd1\x63\xce\x96\x63\x6e\x79\x5c\x0b\x95\x5a\xbd\xfe\x61\xf1\x86\x98\xc1\xba\xc0\x04\x1d\x31\x28\xd4\x00\x00\x87\x4f\x26\x54\x08\xca\x59\x55\xd4\x72\xe6\x57\xde\x35\x25\x81\xde\x92\x64\x33\x97\xa7\x7c\xa2\x10\x6a\xa7\xf2\x5a\x92\x19\x1f\x07\x84\x49\x7b\xcc\xb9\xab\x13\xb4\xf8\x5c\x7e\xaf\x25\x52\x9c\x31\xf5\xdc\x26\x8e\x54\xd8\x51\xf2\xb6\xf6\xf6\x76\xcc\x6e\x7f\x59\xcc\xc5\xeb\x8d\xc2\x2d\x0d\xf0\xcc\x26\x84\x15\xdf\x72\x6d\x6c\x25\xab\x5a\xf4\x03\xea\x94\xf5\xe2\x12\xe1\x04\xd4\x97\x2a\x03\xa5\x84\x0a\x9f\x10\xb7\x32\x69\x89\x00\x2a\xd0\x36\x87\xe5\x70\x26\x03\xec\x34\xc6\xb5\x4a\x96\x8e\xea\xd9\x15\xb2\x74\x49\x74\xf0\x57\xdd\xc1\xf6\x17\xdd\x41\xf9\xe0\x7b\xe4\xe6\x1e\xe8\xb0\xc9\x29\x71\x1a\xf6\xbf\x6c\xb5\xab\xab\x74\xc3\x90\x39\x1a\x44\xb6\x0c\xdd\x3f\x6c\xca\xfc\x50\x02\x2c\x64\x01\xc0\x43\xa9\x17\xe2\xcf\xd9\x82\xfb\x21\xe5\x36\x2e\x6d\x77\x7a\x0c\xe8\x08\x7b\xaa\x59\x43\x01\x26\x72\xb8\x90\xba\x3f\xe4\xd7\xff\x45\xfa\xe2\xf8\x42\x5f\x55\x66\x3b\xbe\xf8\xff\x1a\xe6\xdd\x1e\xdf\x7f\x10\xe0\x94\xcb\x86\xec\x55\x54\x86\xfc\x08\xa8\x8f\xca\x0e\x03\xcd\xb0\x4d\x1c\x5d\xa3\xe3\xba\x01\x25\x52\xb4\xe4\x57\xf8\xc4\x72\xcd\x39\x18\x2d\x34\x22\x8c\x04\xd8\x4b\xd7\x0b\x31\xab\x12\x5b\xd5\xe3\xb5\x41\x83\x29\x3c\xce\x7a\xca\x8d\xa6\x4f\xcc\x51\x03\xf1\xd0\xe3\x27\xf7\x55\x91\x05\x0f\x03\x87\x34\xe9\xa7\x78\xc2\x43\x26\xab\x06\x07\xd5\x69\x03\xe2\x52\x59\xd1\x6a\x93\x29\xa6\x58\xc4\xd4\xe8\x54\x5c\x93\xa7\xd5\x33\x13\x76\x24\x9d\x56\xce\x53\xab\x29\x35\x92\xe8\x0c\xea\x9e\x46\x04\x2f\x63\x78\x82\x83\xe3\x54\xd9\x31\x7d\xa9\x06\x0a\xfc\xc5\xca\xac\xa0\x45\x1f\x52\xa9\x67\x80\x55\xc2\x5d\x14\xeb\x55\x4e\x75\xda\x05\x58\x21\xde\x68\x40\xb8\x9b\x7a\x51\xfc\xee\xb3\xad\xba\xfc\x9c\x2a\xf3\x5d\xfc\x86\xbb\xc0\x9f\x26\x29\x41\xab\x33\x94\xfc\x74\xf0\x18\x9e\xcc\xc4\xd7\x54\x2d\x80\x90\x3c\x20\x70\x4c\x66\x9d\x29\xf6\x42\x15\xf7\x90\x7f\x04\x1e\x3d\x26\x8b\x29\xdd\xe1\x6c\x48\x47\xcb\x92\x7b\x4c\x66\x53\x1c\x7d\x53\xa1\x92\x4c\x92\x79\x63\xe3\x41\x9e\x1c\xe5\x04\x96\xf9\xbc\x7e\x6e\x80\xb6\x3a\x40\x05\xfb\x4f\x00\x00\x00\xff\xff\xd1\x0d\xbb\xcc\xe7\x10\x00\x00")

func NerthusSqlBytes() ([]byte, error) {
	return bindataRead(
		_NerthusSql,
		"../nerthus.sql",
	)
}

func NerthusSql() (*asset, error) {
	bytes, err := NerthusSqlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "../nerthus.sql", size: 4327, mode: os.FileMode(420), modTime: time.Unix(1516103431, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

// Asset loads and returns the asset for the given name.
// It returns an error if the asset could not be found or
// could not be loaded.
func Asset(name string) ([]byte, error) {
	cannonicalName := strings.Replace(name, "\\", "/", -1)
	if f, ok := _bindata[cannonicalName]; ok {
		a, err := f()
		if err != nil {
			return nil, fmt.Errorf("Asset %s can't read by error: %v", name, err)
		}
		return a.bytes, nil
	}
	return nil, fmt.Errorf("Asset %s not found", name)
}

// MustAsset is like Asset but panics when Asset would return an error.
// It simplifies safe initialization of global variables.
func MustAsset(name string) []byte {
	a, err := Asset(name)
	if err != nil {
		panic("asset: Asset(" + name + "): " + err.Error())
	}

	return a
}

// AssetInfo loads and returns the asset info for the given name.
// It returns an error if the asset could not be found or
// could not be loaded.
func AssetInfo(name string) (os.FileInfo, error) {
	cannonicalName := strings.Replace(name, "\\", "/", -1)
	if f, ok := _bindata[cannonicalName]; ok {
		a, err := f()
		if err != nil {
			return nil, fmt.Errorf("AssetInfo %s can't read by error: %v", name, err)
		}
		return a.info, nil
	}
	return nil, fmt.Errorf("AssetInfo %s not found", name)
}

// AssetNames returns the names of the assets.
func AssetNames() []string {
	names := make([]string, 0, len(_bindata))
	for name := range _bindata {
		names = append(names, name)
	}
	return names
}

// _bindata is a table, holding each asset generator, mapped to its name.
var _bindata = map[string]func() (*asset, error){
	"../nerthus.sql": NerthusSql,
}

// AssetDir returns the file names below a certain
// directory embedded in the file by go-bindata.
// For example if you run go-bindata on data/... and data contains the
// following hierarchy:
//     data/
//       foo.txt
//       img/
//         a.png
//         b.png
// then AssetDir("data") would return []string{"foo.txt", "img"}
// AssetDir("data/img") would return []string{"a.png", "b.png"}
// AssetDir("foo.txt") and AssetDir("notexist") would return an error
// AssetDir("") will return []string{"data"}.
func AssetDir(name string) ([]string, error) {
	node := _bintree
	if len(name) != 0 {
		cannonicalName := strings.Replace(name, "\\", "/", -1)
		pathList := strings.Split(cannonicalName, "/")
		for _, p := range pathList {
			node = node.Children[p]
			if node == nil {
				return nil, fmt.Errorf("Asset %s not found", name)
			}
		}
	}
	if node.Func != nil {
		return nil, fmt.Errorf("Asset %s not found", name)
	}
	rv := make([]string, 0, len(node.Children))
	for childName := range node.Children {
		rv = append(rv, childName)
	}
	return rv, nil
}

type bintree struct {
	Func     func() (*asset, error)
	Children map[string]*bintree
}

var _bintree = &bintree{nil, map[string]*bintree{
	"..": &bintree{nil, map[string]*bintree{
		"nerthus.sql": &bintree{NerthusSql, map[string]*bintree{}},
	}},
}}

// RestoreAsset restores an asset under the given directory
func RestoreAsset(dir, name string) error {
	data, err := Asset(name)
	if err != nil {
		return err
	}
	info, err := AssetInfo(name)
	if err != nil {
		return err
	}
	err = os.MkdirAll(_filePath(dir, filepath.Dir(name)), os.FileMode(0755))
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(_filePath(dir, name), data, info.Mode())
	if err != nil {
		return err
	}
	err = os.Chtimes(_filePath(dir, name), info.ModTime(), info.ModTime())
	if err != nil {
		return err
	}
	return nil
}

// RestoreAssets restores an asset under the given directory recursively
func RestoreAssets(dir, name string) error {
	children, err := AssetDir(name)
	// File
	if err != nil {
		return RestoreAsset(dir, name)
	}
	// Dir
	for _, child := range children {
		err = RestoreAssets(dir, filepath.Join(name, child))
		if err != nil {
			return err
		}
	}
	return nil
}

func _filePath(dir, name string) string {
	cannonicalName := strings.Replace(name, "\\", "/", -1)
	return filepath.Join(append([]string{dir}, strings.Split(cannonicalName, "/")...)...)
}
