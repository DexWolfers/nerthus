// Author: @ysqi

package cmd

import (
	"os"
	"runtime"
	"testing"
	"time"

	"gitee.com/nerthus/nerthus/app/utils"
	"gitee.com/nerthus/nerthus/params"
)

var (
	modules = "admin:1.0 debug:1.0 net:1.0 nts:1.0 personal:1.0 rpc:1.0 solc:1.0 web3:1.0 wist:1.0"
)

// Tests that a node embedded within a console can be started up properly and
// then terminated by closing the input stream.
func TestConsoleWelcome(t *testing.T) {
	mainaccount := "0x7ef5a6135f1fd6a02593eedc869c6d41d934aef8"
	datadir := tmpDatadirWithKeystore(t)
	defer os.RemoveAll(datadir)
	// Start a geth console, make sure it's cleaned up and terminate the console
	geth := runMain(t,
		"--port", "0", "--maxpeers", "0", "--mainaccount", mainaccount, "--datadir", datadir,
		"console")
	// Gather all the infos the welcome message needs to contain
	geth.SetTemplateFunc("goos", func() string { return runtime.GOOS })
	geth.SetTemplateFunc("goarch", func() string { return runtime.GOARCH })
	geth.SetTemplateFunc("gover", runtime.Version)
	geth.SetTemplateFunc("gethver", func() string { return params.Version })
	geth.SetTemplateFunc("modules", func() string { return modules })
	// Verify the actual welcome message to the required template
	geth.Expect(`
Welcome to the cnts JavaScript console!

 instance: cnts/v{{gethver}}/{{goos}}-{{goarch}}/{{gover}}
 mainaccount: {{.D.mainaccount}}
 datadir: {{.D.datadir}}
 modules: {{modules}}

> {{.InputLine "exit"}}
`)

	geth.ExpectExit()
}

// 测试控制台交易发送
// Author: @ysqi
func TestConsoleSendTranscation(t *testing.T) {
	mainaccount := "0x7ef5a6135f1fd6a02593eedc869c6d41d934aef8"
	datadir := tmpDatadirWithKeystore(t)
	defer os.RemoveAll(datadir)

	//单个
	t.Run("one2one", func(t *testing.T) {
		cnts := runMain(t, "--maxpeers", "0", "--port", "0", "--nodiscover", "--mainaccount", mainaccount, "--datadir", datadir, "--dev", "console")
		time.Sleep(time.Second * 3)
		cnts.InputLine("nts.accounts")
		cnts.InputLine("personal.unlockAccount(nts.accounts[0], 'foobar')")
		cnts.InputLine("tx={from:nts.accounts[0],to:nts.mainAccount,amount:10, gasPrice:1}")
		cnts.InputLine("nts.sendTransaction(tx)")
		cnts.ExpectRegexp(`"0x[0-9a-f]{64}"\n`) // 交易单元哈希
	})
	//多个
	//t.Run("one2more", func(t *testing.T) {
	//	cnts := runMain(t, "--maxpeers", "0", "--port", "0", "--mainaccount", mainaccount, "--datadir", datadir, "console")
	//	cnts.InputLine("personal.unlockAccount(nts.mainAccount, 'foobar')")
	//	cnts.InputLine("tx={from:nts.mainAccount,to:[nts.accounts[0],nts.accounts[1],nts.accounts[2]],amount:[100,88,77]}")
	//	cnts.InputLine("nts.sendTransaction(tx)")
	//	cnts.InputLine("exit")
	//	cnts.ExpectRegexp(`"0x[0-9a-f]{64}"\n`) // 交易单元哈希
	//})

}

// Author: @zwj 测试见证人更换
func TestConsoleWitnessReplace(t *testing.T) {
	mainaccount := "0x7ef5a6135f1fd6a02593eedc869c6d41d934aef8"
	datadir := tmpDatadirWithKeystore(t)
	defer os.RemoveAll(datadir)

	cnts := runMain(t, "--maxpeers", "0", "--port", "0", "--mainaccount", mainaccount, "--datadir", datadir, "console")
	cnts.InputLine("acc=nts.mainAccount")
	cnts.InputLine("tx={from:acc}")
	cnts.InputLine("nts.replaceWitness(tx,'foobar')")
	cnts.InputLine("exit")
	//cnts.ExpectRegexp(`"0x[0-9a-f]{64}"\n`) // 交易单元哈希
}

// Author: @zwj 测试注销见证人
func TestConsoleWitnessCancel(t *testing.T) {
	mainaccount := "0x7ef5a6135f1fd6a02593eedc869c6d41d934aef8"
	datadir := tmpDatadirWithKeystore(t)
	defer os.RemoveAll(datadir)

	t.Run("", func(t *testing.T) {
		cnts := runMain(t, "--maxpeers", "0", "--port", "0", "--mainaccount", mainaccount, "--datadir", datadir, "console")
		cnts.InputLine("acc=nts.mainAccount")
		cnts.InputLine("tx={from:acc}")
		cnts.InputLine("nts.retrieveWitness(tx, 'foobar')")
		cnts.InputLine("exit")
	})
}

// Author: @zwj 测试举报见证人
func TestConsoleWitnessReport(t *testing.T) {
	mainaccount := "0x7ef5a6135f1fd6a02593eedc869c6d41d934aef8"
	datadir := tmpDatadirWithKeystore(t)
	defer os.RemoveAll(datadir)

	t.Run("", func(t *testing.T) {
		cnts := runMain(t, "--maxpeers", "0", "--port", "0", "--mainaccount", mainaccount, "--datadir", datadir, "console")
		cnts.InputLine("acc=nts.mainAccount")
		cnts.InputLine("hash='0x0000000000000000000000000000000000000000000abc000000000000000234'")
		cnts.InputLine("tx={from:acc, hash:hash}")
		cnts.InputLine("nts.reportWitness(tx, 'foobar')")
		cnts.ExpectRegexp(`"0x[0-9a-f]{64}"\n`)
	})
}

// Author: @zwj 生成见证人
func TestConsoleWitnessGenerate(t *testing.T) {
	mainaccount := "0x7ef5a6135f1fd6a02593eedc869c6d41d934aef8"
	datadir := tmpDatadirWithKeystore(t)
	defer os.RemoveAll(datadir)

	t.Run("", func(t *testing.T) {
		cnts := runMain(t, "--maxpeers", "0", "--port", "0", "--mainaccount", mainaccount, "--datadir", datadir, "console")
		cnts.InputLine("acc=nts.mainAccount")
		cnts.InputLine("tx={from:acc}")
		cnts.InputLine("nts.generate(tx, 'foobar')")
		cnts.InputLine(`"0x[0-9a-f]{64}"\n`)
	})
}

// 获取见证人合约地址
// Author: @zwj
func TestConsoleGetWMC(t *testing.T) {
	mainaccount := "0x7ef5a6135f1fd6a02593eedc869c6d41d934aef8"
	datadir := tmpDatadirWithKeystore(t)
	defer os.RemoveAll(datadir)

	t.Run("", func(t *testing.T) {
		cnts := runMain(t, "--maxpeers", "0", "--port", "0", "--mainaccount", mainaccount, "--datadir", datadir, "console")
		cnts.InputLine("nts.wmcAccount")
		cnts.InputLine(`"0x[0-9a-f]{64}"\n`)
	})
}

func TestConsoleAPI_EstimateGas(t *testing.T) {
	mainaccount := "0x7ef5a6135f1fd6a02593eedc869c6d41d934aef8"
	t.Run("transfer", func(t *testing.T) {
		datadir := tmpDatadirWithKeystore(t)
		defer os.RemoveAll(datadir)
		cnts := runMain(t, "--maxpeers", "0", "--port", "0", "--mainaccount", mainaccount, "--datadir", datadir, "console")
		cnts.InputLine("acc=nts.mainAccount")
		cnts.InputLine("nts.estimateGas({from:acc,to:acc,amount:1000})")
		cnts.ExpectRegexp(`\d+`)
	})
	t.Run("createContract", func(t *testing.T) {
		datadir := tmpDatadirWithKeystore(t)
		defer os.RemoveAll(datadir)
		cnts := runMain(t, "--maxpeers", "0", "--port", "0", "--mainaccount", mainaccount, "--datadir", datadir, "console")
		cnts.InputLine("acc=nts.mainAccount")
		// contract MyC {}
		cnts.InputLine("nts.estimateGas({from:acc,amount:1000, gasPrice:1,gas:3,data:'0x60606040523415600e57600080fd5b603580601b6000396000f3006060604052600080fd00a165627a7a72305820c7c3b99ffa3a1875bbba04c4a71d217ae9773217eca0c64ec50883818917980b0029' })")
		cnts.ExpectRegexp(`\d`) // 结果：usedgas value
	})
}

// 网络参数测试
// Author: @ysqi
func TestConsole_NetworkIP(t *testing.T) {
	t.Run("customNetwork_default", func(t *testing.T) {
		cnts := runMain(t, "--maxpeers", "0", "console")
		cnts.InputLine("admin.nodeInfo.listenAddr")
		cnts.ExpectRegexp(`\[::\]:60101`)
	})
	t.Run("customNetwork_ip", func(t *testing.T) {
		cnts := runMain(t, "--maxpeers", "0", "--ip", "127.0.0.1", "console")
		cnts.InputLine("admin.nodeInfo.listenAddr")
		cnts.ExpectRegexp(`127\.0\.0\.1:60101`)
	})
	t.Run("customNetwork_ip_port", func(t *testing.T) {
		cnts := runMain(t, "--maxpeers", "0", "--port", "60102", "--ip", "127.0.0.1", "console")
		cnts.InputLine("admin.nodeInfo.listenAddr")
		cnts.ExpectRegexp(`127\.0\.0\.1:60102`)
	})
	t.Run("customNetwork_dev_default", func(t *testing.T) {
		ip, err := utils.GetLocalIP4()
		if err != nil {
			t.Fatal(err)
		}
		cnts := runMain(t, "--maxpeers", "0", "--port", "60102", "--dev", "console")
		cnts.InputLine("admin.nodeInfo.listenAddr")
		cnts.ExpectRegexp(ip + ":" + "60102")
	})
	t.Run("customNetwork_dev", func(t *testing.T) {
		cnts := runMain(t, "--maxpeers", "0", "--port", "60102", "--ip", "127.0.0.1", "--dev", "console")
		cnts.InputLine("admin.nodeInfo.listenAddr")
		cnts.ExpectRegexp(`127\.0\.0\.1:60102`)
	})
}
