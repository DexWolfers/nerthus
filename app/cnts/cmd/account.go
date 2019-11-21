// Author: @yin

package cmd

import (
	"fmt"
	"io"
	"os"
	"strconv"

	"gitee.com/nerthus/nerthus/accounts"
	"gitee.com/nerthus/nerthus/accounts/keystore"
	"gitee.com/nerthus/nerthus/app/utils"
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/console"
	"gitee.com/nerthus/nerthus/crypto"
	"gitee.com/nerthus/nerthus/log"

	"github.com/spf13/cobra"
)

// accountCmd represents the account command
var accountCmd = &cobra.Command{
	Use:   "account",
	Short: "manage accounts",
	Long: `Manage accounts, list all existing accounts, import a private key into a new
account, create a new account or update an existing account.

It supports interactive mode, when you are prompted for password as well as
non-interactive mode where passwords are supplied via a given password file.
Non-interactive mode is only meant for scripted use on test networks or known
safe environments.

Make sure you remember the password you gave when creating a new account (with
either new or import). Without it you are not able to unlock your account.

Note that exporting your key in unencrypted format is NOT supported.

Keys are stored under <DATADIR>/[tesnet][dev][main]/keystore.
It is safe to transfer the entire directory or the individual keys therein
between nerthus nodes by simply copying.

Make sure you backup your keys regularly.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("a brief description of your command")
	},
}

// list
var accountListCmd = &cobra.Command{
	Use:   "list",
	Short: "Print summary of existing accounts",
	Long:  `Print a short summary of all accounts`,
	Args:  cobra.MaximumNArgs(0),
	Run:   accountList,
}

// create
var accountCreateCmd = &cobra.Command{
	Use:   "new",
	Short: "Create a new account",
	Long: `cnts account new

Creates a new account and prints the address.

The account is saved in encrypted format, you are prompted for a passphrase.

You must remember this passphrase to unlock your account in the future.

For non-interactive use the passphrase can be specified with the --password flag:

Note, this is meant to be used for testing only, it is a bad idea to save your
password to file or expose in any other way.`,
	Args: cobra.MaximumNArgs(1),
	Run:  accountCreate,
}

//update
var accountUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update an existing account",
	Long: `cnts account update <address>

Update an existing account.

The account is saved in the newest version in encrypted format, you are prompted
for a passphrase to unlock the account and another to save the updated file.

This same command can therefore be used to migrate an account of a deprecated
format to the newest format or change the password for an account.

For non-interactive use the passphrase can be specified with the --password flag:

    cnts account update [options] <address>
.
For interactive use only can specific addresses more than 3 addresses.`,
	Args: cobra.MaximumNArgs(3),
	Run:  updateAccount,
}

// import
var accountImportCmd = &cobra.Command{
	Use:   "import",
	Short: "Import a private key into a new account",
	Long: `cnts account import <keyfile>

Imports an unencrypted private key from <keyfile> and creates a new account.
Prints the address.

The keyfile is assumed to contain an unencrypted private key in hexadecimal format.

The account is saved in encrypted format, you are prompted for a passphrase.

You must remember this passphrase to unlock your account in the future.

For non-interactive use the passphrase can be specified with the -password flag:

    cnts account import [options] <keyfile>

Note:
As you can directly copy your encrypted accounts to another Nerthus instance,
this import mechanism is not needed when you transfer an account between
nodes.`,
	Args: cobra.MaximumNArgs(1),
	Run:  importAccount,
}

func init() {

	accountListCmd.Flags().AddFlag(utils.DataDirFlag)
	accountListCmd.Flags().AddFlag(utils.KeyStoreDirFlag)

	accountCreateCmd.Flags().AddFlag(utils.DataDirFlag)
	accountCreateCmd.Flags().AddFlag(utils.KeyStoreDirFlag)
	accountCreateCmd.Flags().AddFlag(utils.PasswordFileFlag)
	accountCreateCmd.Flags().AddFlag(utils.LightKDFFlag)

	accountUpdateCmd.Flags().AddFlag(utils.DataDirFlag)
	accountUpdateCmd.Flags().AddFlag(utils.KeyStoreDirFlag)
	accountUpdateCmd.Flags().AddFlag(utils.PasswordFileFlag)
	accountUpdateCmd.Flags().AddFlag(utils.LightKDFFlag)

	accountImportCmd.Flags().AddFlag(utils.DataDirFlag)
	accountImportCmd.Flags().AddFlag(utils.KeyStoreDirFlag)
	accountImportCmd.Flags().AddFlag(utils.PasswordFileFlag)
	accountImportCmd.Flags().AddFlag(utils.LightKDFFlag)

	RootCmd.AddCommand(accountCmd)
	accountCmd.AddCommand(
		accountListCmd,
		accountCreateCmd,
		accountUpdateCmd,
		accountImportCmd,
	)

}

// accountList 列出keystore目录下的所有账户地址
func accountList(cmd *cobra.Command, args []string) {
	stack, _ := makeConfigNode(&utils.Flags{FlagSet: RootCmd.Flags()})
	var index int
	for _, wallet := range stack.AccountManager().Wallets() {
		for _, account := range wallet.Accounts() {
			fmt.Printf("Account #%d: {%x} %s\n", index, account.Address, &account.URL)
			index++
		}
	}
}

// accountCreate 生成账户
func accountCreate(cmd *cobra.Command, args []string) {
	flags := &utils.Flags{FlagSet: RootCmd.Flags()}
	stack, _ := makeConfigNode(flags)
	ks := stack.AccountManager().Backends(keystore.KeyStoreType)[0].(*keystore.KeyStore)
	var password string
	if passwordFile := flags.GetString(utils.PasswordFileFlag.Name); passwordFile != "" {
		var err error
		//从文件读取密码
		password, err = readPasswordByFile(passwordFile)
		if err != nil {
			if io.EOF == err {
				utils.Fatalf("Empty content in %s", passwordFile)
			}
			utils.Fatalf("Read password content failed,%v", err)
		}
	} else {

		// @ysqi: there is only one arg at most. checked by cobra, see accountCreateCmd define.
		if len(args) == 0 {
			password = getPassPhrase("Your new account is locked with a password. Please give a password. Do not forget this password.", true, 0, nil)
		} else {
			password = args[0]
		}
	}

	account, err := ks.NewAccount(password)
	if err != nil {
		utils.Fatalf("Failed to create account: %v", err)
	}
	// ks.NewAccount 内部已做检查，这里不应出现
	if account.Address.IsContract() {
		ks.Delete(account, password)
		utils.Fatalf("Failed to create account: address {%x} contains contract address flag. Please report issue as a bug.", account.Address)
	}
	fmt.Printf("Address: {%x}\n", account.Address)
}

// updateAccount 更新密码
func updateAccount(cmd *cobra.Command, args []string) {
	stack, _ := makeConfigNode(&utils.Flags{FlagSet: RootCmd.Flags()})
	ks := stack.AccountManager().Backends(keystore.KeyStoreType)[0].(*keystore.KeyStore)
	for _, addr := range args {
		account, oldPassword := unlockAccount(ks, addr, 0, nil)
		newPassword := getPassPhrase("Please give a new password. Do not forget this password.", true, 0, nil)
		if err := ks.Update(account, oldPassword, newPassword); err != nil {
			utils.Fatalf("Could not update the account: %v", err)
		}
	}
}

// importAccount 导入私钥 从文件导入
func importAccount(cmd *cobra.Command, args []string) {
	if len(args) == 0 {
		utils.Fatalf("keyfile must be given as argument")
	}
	key, err := crypto.LoadECDSA(args[0])
	if err != nil {
		utils.Fatalf("Failed to load the private key: %v", err)
	}
	flags := &utils.Flags{FlagSet: RootCmd.Flags()}
	stack, _ := makeConfigNode(flags)
	passphrase := getPassPhrase("Your new account is locked with a password. Please give a password. Do not forget this password.", true, 0, utils.MakePasswordList(flags))

	ks := stack.AccountManager().Backends(keystore.KeyStoreType)[0].(*keystore.KeyStore)
	acct, err := ks.ImportECDSA(key, passphrase)
	if err != nil {
		utils.Fatalf("Could not create the account: %v", err)
	}
	fmt.Printf("Address: {%x}\n", acct.Address)
}

func unlockAccount(ks *keystore.KeyStore, address string, i int, passwords []string) (accounts.Account, string) {
	account, err := makeAddress(ks, address)
	if err != nil {
		utils.Fatalf("Could not list accounts: %v", err)
	}
	for trials := 0; trials < 3; trials++ {
		prompt := fmt.Sprintf("Unlocking account %s | Attempt %d/%d", address, trials+1, 3)
		password := getPassPhrase(prompt, false, i, passwords)
		err = ks.Unlock(account, password)
		if err == nil {
			log.Info("Unlocked account", "address", account.Address.Hex())
			return account, password
		}
		if err, ok := err.(*keystore.AmbiguousAddrError); ok {
			log.Info("Unlocked account", "address", account.Address.Hex())
			return ambiguousAddrRecovery(ks, err, password), password
		}
		if err != keystore.ErrDecrypt {
			// No need to prompt again if the error is not decryption-related.
			break
		}
	}
	// All trials expended to unlock account, bail out
	utils.Fatalf("Failed to unlock account %s (%v)", address, err)

	return accounts.Account{}, ""
}

func makeAddress(ks *keystore.KeyStore, account string) (accounts.Account, error) {
	// If the specified account is a valid address, return it
	if addr, err := common.DecodeAddress(account); err == nil {
		return accounts.Account{Address: addr}, nil
	}

	// Otherwise try to interpret the account as a keystore index
	index, err := strconv.Atoi(account)
	if err != nil || index < 0 {
		return accounts.Account{}, fmt.Errorf("invalid account address or index %q", account)
	}
	accs := ks.Accounts()
	if len(accs) <= index {
		return accounts.Account{}, fmt.Errorf("index %d higher than number of accounts %d", index, len(accs))
	}
	return accs[index], nil
}

// ambiguousAddrRecovery 多个密码文件存在
func ambiguousAddrRecovery(ks *keystore.KeyStore, err *keystore.AmbiguousAddrError, auth string) accounts.Account {
	fmt.Printf("Multiple key files exist for address %x:\n", err.Addr)
	for _, a := range err.Matches {
		fmt.Println("  ", a.URL)
	}
	fmt.Println("Testing your passphrase against all of them...")
	var match *accounts.Account
	for _, a := range err.Matches {
		if err := ks.Unlock(a, auth); err == nil {
			match = &a
			break
		}
	}
	if match == nil {
		utils.Fatalf("None of the listed files could be unlocked.")
	}
	fmt.Printf("Your passphrase unlocked %s\n", match.URL)
	fmt.Println("In order to avoid this warning, you need to remove the following duplicate key files:")
	for _, a := range err.Matches {
		if a != *match {
			fmt.Println("  ", a.URL)
		}
	}
	return *match
}

// getPassPhrase 从控制台获取用户输入的密码信息
func getPassPhrase(prompt string, confirmation bool, i int, passwords []string) string {
	// If a list of passwords was supplied, retrieve from them
	if len(passwords) > 0 {
		if i < len(passwords) {
			return passwords[i]
		}
		return passwords[len(passwords)-1]
	}
	// Otherwise prompt the user for the password
	if prompt != "" {
		fmt.Println(prompt)
	}
	password, err := console.Stdin.PromptPassword("Passphrase: ")
	if err != nil {
		utils.Fatalf("Failed to read passphrase: %v", err)
	}
	if confirmation {
		confirm, err := console.Stdin.PromptPassword("Repeat passphrase: ")
		if err != nil {
			utils.Fatalf("Failed to read passphrase confirmation: %v", err)
		}
		if password != confirm {
			utils.Fatalf("Passphrases do not match")
		}
	}
	return password
}

// 从文件读取 密码
func readPasswordByFile(file string) (string, error) {
	buf := make([]byte, 64)
	fd, err := os.Open(file)
	if err != nil {
		return "", err
	}
	defer fd.Close()
	if _, err := io.ReadAtLeast(fd, buf, 1); err != nil {
		return "", err
	}
	return string(buf), nil
}
