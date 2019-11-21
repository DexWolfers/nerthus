package cmd

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"gitee.com/nerthus/nerthus/crypto"
)

func tmpDatadirWithKeystore(t *testing.T) string {
	datadir := tmpdir(t)
	keystore := filepath.Join(datadir, "keystore")
	source := filepath.Join("..", "..", "..", "accounts", "keystore", "testdata", "keystore")
	if err := copyAll(keystore, source); err != nil {
		os.RemoveAll(datadir)
		t.Fatal(err)
	}
	return datadir
}

func TestAccountListEmpty(t *testing.T) {
	cnts := runMain(t, "account", "list")
	cnts.ExpectExit()
}

func TestAccountNew(t *testing.T) {

	t.Run("default", func(t *testing.T) {
		cnts := runMain(t, "account", "new", "--lightkdf")
		defer cnts.ExpectExit()
		cnts.Expect(`
Your new account is locked with a password. Please give a password. Do not forget this password.
!! Unsupported terminal, password will be echoed.
Passphrase: {{.InputLine "foobar"}}
Repeat passphrase: {{.InputLine "foobar"}}
`)
		cnts.ExpectRegexp(`Address: \{nts1[0-9a-z]+\}\n`)
	})

	t.Run("badpw", func(t *testing.T) {

		t.Run("diff", func(t *testing.T) {
			cnts := runMain(t, "account", "new", "--lightkdf")
			defer cnts.ExpectExit()
			cnts.Expect(`
Your new account is locked with a password. Please give a password. Do not forget this password.
!! Unsupported terminal, password will be echoed.
Passphrase: {{.InputLine "something"}}
Repeat passphrase: {{.InputLine "something else"}}
Fatal: Passphrases do not match
`)

		})

		t.Run("more", func(t *testing.T) {
			cnts := runMain(t, "account", "new", "pw1", "pw2")
			cnts.WaitExit()
			cnts.ErrExpect(`Error: accepts at most 1 arg(s), received 2`)
		})

	})

	t.Run("passwordfile", func(t *testing.T) {
		t.Run("notexist", func(t *testing.T) {
			cnts := runMain(t, "account", "new", "--password", "./a/b/c/d/not/exist/pw.txt")
			cnts.WaitExit()
			cnts.ErrExpect(`Error: invalid argument "./a/b/c/d/not/exist/pw.txt" for "--password" flag`)
		})
		t.Run("empty", func(t *testing.T) {
			file := tmpfile(t)
			cnts := runMain(t, "account", "new", "--password", file)
			cnts.WaitExit()
			cnts.ErrExpect(`Fatal: Empty content in ` + file)
		})
		t.Run("exist", func(t *testing.T) {
			file := tmpfile(t)
			if err := ioutil.WriteFile(file, []byte("newpassword"), os.ModeAppend); err != nil {
				t.Fatal(err)
			}
			cnts := runMain(t, "account", "new", "--password", file)
			defer cnts.ExpectExit()
			cnts.Cleanup = func() { os.Remove(file) }

			cnts.ExpectRegexp(`Address: \{nts1[0-9a-z]+\}\n`)
		})
	})

}

func TestAccountList(t *testing.T) {

	t.Run("good", func(t *testing.T) {
		datadir := tmpDatadirWithKeystore(t)
		defer os.RemoveAll(datadir)

		cnts := runMain(t, "account", "list", "--datadir", datadir)
		defer cnts.ExpectExit()
		if runtime.GOOS == "windows" {
			cnts.Expect(`
Account #0: {nts10m66vy6lrlt2qfvnamwgd8rdg8vnfthc3v4ue9} keystore://{{.D.datadir}}\keystore\UTC--2016-03-22T12-57-55.920751759Z--7ef5a6135f1fd6a02593eedc869c6d41d934aef8
Account #1: {nts173ngt84dryedws7kyt9hflq93zpwsey28awaf4} keystore://{{.D.datadir}}\keystore\aaa
Account #2: {nts19zw5shvhw9c5en536vun6ajwzvgeq7kvtvran8} keystore://{{.D.datadir}}\keystore\zzz
`)
		} else {
			cnts.Expect(`
Account #0: {nts10m66vy6lrlt2qfvnamwgd8rdg8vnfthc3v4ue9} keystore://{{.D.datadir}}/keystore/UTC--2016-03-22T12-57-55.920751759Z--7ef5a6135f1fd6a02593eedc869c6d41d934aef8
Account #1: {nts173ngt84dryedws7kyt9hflq93zpwsey28awaf4} keystore://{{.D.datadir}}/keystore/aaa
Account #2: {nts19zw5shvhw9c5en536vun6ajwzvgeq7kvtvran8} keystore://{{.D.datadir}}/keystore/zzz
`)
		}
	})

	t.Run("empty", func(t *testing.T) {
		datadir := tmpdir(t)
		defer os.RemoveAll(datadir)

		cnts := runMain(t, "account", "list", "--datadir", datadir)
		defer cnts.ExpectExit()
		cnts.Expect("")
	})

	t.Run("keystorefile", func(t *testing.T) {
		datadir := tmpdir(t)
		defer os.RemoveAll(datadir)

		dir := tmpDatadirWithKeystore(t)
		defer os.RemoveAll(dir)
		keystore := filepath.Join(dir, "keystore")

		cnts := runMain(t, "account", "list", "--datadir", datadir, "--keystore", keystore)
		defer cnts.ExpectExit()
		if runtime.GOOS == "windows" {
			cnts.Expect(`
Account #0: {nts10m66vy6lrlt2qfvnamwgd8rdg8vnfthc3v4ue9} keystore://{{.D.datadir}}\keystore\UTC--2016-03-22T12-57-55.920751759Z--7ef5a6135f1fd6a02593eedc869c6d41d934aef8
Account #1: {nts173ngt84dryedws7kyt9hflq93zpwsey28awaf4} keystore://{{.D.datadir}}\keystore\aaa
Account #2: {nts19zw5shvhw9c5en536vun6ajwzvgeq7kvtvran8} keystore://{{.D.datadir}}\keystore\zzz
`)
		} else {
			cnts.Expect(`
Account #0: {nts10m66vy6lrlt2qfvnamwgd8rdg8vnfthc3v4ue9} keystore://{{.D.datadir}}/keystore/UTC--2016-03-22T12-57-55.920751759Z--7ef5a6135f1fd6a02593eedc869c6d41d934aef8
Account #1: {nts173ngt84dryedws7kyt9hflq93zpwsey28awaf4} keystore://{{.D.datadir}}/keystore/aaa
Account #2: {nts19zw5shvhw9c5en536vun6ajwzvgeq7kvtvran8} keystore://{{.D.datadir}}/keystore/zzz
`)
		}
	})

}

func TestAccountUpdate(t *testing.T) {
	datadir := tmpDatadirWithKeystore(t)

	cnts := runMain(t, "account", "update",
		"--datadir", datadir, "--lightkdf",
		"nts10m66vy6lrlt2qfvnamwgd8rdg8vnfthc3v4ue9")
	defer cnts.ExpectExit()
	cnts.Expect(`
Unlocking account nts10m66vy6lrlt2qfvnamwgd8rdg8vnfthc3v4ue9 | Attempt 1/3
!! Unsupported terminal, password will be echoed.
Passphrase: {{.InputLine "foobar"}}
Please give a new password. Do not forget this password.
Passphrase: {{.InputLine "foobar2"}}
Repeat passphrase: {{.InputLine "foobar2"}}
`)
}

func TestAccountImport(t *testing.T) {

	t.Run("bad", func(t *testing.T) {
		cnts := runMain(t, "account", "import")
		defer cnts.ExpectExit()
		cnts.WaitExit()
		cnts.ErrExpect("Fatal: keyfile must be given as argument")
	})
	t.Run("good", func(t *testing.T) {
		file := tmpfile(t)
		key, _ := crypto.GenerateKey()
		err := crypto.SaveECDSA(file, key)
		if err != nil {
			t.Fatal(err)
		}
		cnts := runMain(t, "account", "import", file)
		defer cnts.ExpectExit()
		cnts.Expect(`
Your new account is locked with a password. Please give a password. Do not forget this password.
!! Unsupported terminal, password will be echoed.
Passphrase: {{.InputLine "foo"}}
Repeat passphrase: {{.InputLine "foo"}}`)
		cnts.ExpectRegexp(`Address: \{nts1[0-9a-z]+\}\n`)

		files, err := ioutil.ReadDir(filepath.Join(cnts.D["datadir"], "testnet", "keystore"))
		if len(files) != 1 {
			t.Errorf("expected one key file in keystore directory, found %d files (error: %v)", len(files), err)
		}
	})
}
