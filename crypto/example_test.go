package crypto

import (
	"encoding/hex"
	"fmt"
	"testing"
)

// @author yin

var pwd = []byte("abc")

func TestGenerateKey(t *testing.T) {
	for i := 50; i > 0; i-- {
		pkey, _ := GenerateKey()
		fmt.Println(fmt.Sprintf("\"%s\",", hex.EncodeToString(FromECDSA(pkey))))
	}
}

//私钥 公钥 地址生成
func TestPka(t *testing.T) {
	//ExampleGenerateKey()
}

// 私钥 公钥 地址生成
func ExampleGenerateKey() {
	//	私钥
	key := Keccak256Hash(pwd)
	fmt.Println("key:", key)
	//16进制的是私钥
	fmt.Println("key-hex:", key.Hex())
	//fmt.Println("key-hex:")
	//fmt.Println("key:",key)
	//去掉16进制形式的私钥
	keyStr := hex.EncodeToString(key.Bytes())
	fmt.Println("keyStr:", keyStr)

	//	转换成ecdsa 生成 PrivateKey
	skey, _ := HexToECDSA(keyStr)
	fmt.Println("skey:", skey)

	//	公钥 65位
	publicKeyBytes := FromECDSAPub(&skey.PublicKey)
	fmt.Println("publicKeyBytes:", publicKeyBytes)

	//	公钥 进行hash 生成32位
	pub := Keccak256(publicKeyBytes[1:])
	fmt.Println("pub:", pub)
	pubString := hex.EncodeToString(pub)
	fmt.Println("pub str:", pubString)

	//	地址 20位
	address := ForceParsePubKeyToAddress(skey.PublicKey)
	fmt.Println("address:", address)
	address_byte := hex.EncodeToString(address.Bytes())
	fmt.Println("address:", address_byte)

	//	msg 进行hash
	msg := Keccak256([]byte("hello"))

	//	对msg进行 私钥签名
	sig, err := Sign(msg, skey)
	if err != nil {
		fmt.Println("err :", err)
		return
	}

	//	认证 签名是否有效  利用 msg sig求出公钥
	//	从签名中获取公钥
	recoveredPub, err := Ecrecover(msg, sig)
	if err != nil {
		fmt.Println("err re:", err)
		return
	}
	fmt.Println("recoveredPub:", recoveredPub)
	fmt.Println("recoveredPub:", hex.EncodeToString(recoveredPub))

	//比较形式有三种 ：

	// A: 65位 进行比较
	if hex.EncodeToString(recoveredPub) == hex.EncodeToString(publicKeyBytes) {
		fmt.Println("OK")
	}

	//	B: 以地址的形式比较
	pubSigKey := ToECDSAPub(recoveredPub)
	sigAddress := ForceParsePubKeyToAddress(*pubSigKey)

	sigAddress_byte := hex.EncodeToString(sigAddress.Bytes())
	fmt.Println("sigAddress:", sigAddress_byte)

	if address_byte == sigAddress_byte {
		fmt.Println("address ok")
	}

	//	将公钥转换为长度为65的字节序列
	//	recoveredPubBytes := FromECDSAPub(recoveredPub)

	sigPublicKey, err := SigToPub(msg, sig)
	if err != nil {
		fmt.Println("sigPu err:", sigPublicKey)
		return
	}

	sigPublicKey65 := FromECDSAPub(sigPublicKey)
	fmt.Println("sigP65:", sigPublicKey65)
	sigPublicKey32 := Keccak256(sigPublicKey65[1:])
	fmt.Println("sigP32", sigPublicKey32)
	sigPublicKey32String := hex.EncodeToString(sigPublicKey32)
	fmt.Println("sigPublicKey32String:", sigPublicKey32String)

	//	C: 32位公钥 比较
	if pubString == sigPublicKey32String {
		fmt.Println("pub32 ok")
	}

}
