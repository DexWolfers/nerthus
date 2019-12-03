#!/bin/bash
ntsCMD="$GOPATH/src/gitee.com/nerthus/nerthus/build/bin/cnts"

baseP2PPort="61101"
baseHTTPRPC="8755"
baseDIR="/tmp/nerthus_cluster"
clusterNumbers=10
nodeKey="nerthusnodekey"
nodeIP=`ifconfig en1 | grep inet | grep -v inet6 | awk '{print $2}'`
#nodeIP="127.0.0.1"
bootNode="enode://ff85755f313aaeff5fc62ae73751224933acf6b32020b6bfb4268b411061e1e8b03c542768b77ba1b71a52958874b4994244d32c765c25872b4aee2ffd026a52@[$nodeIP]:$baseP2PPort"

echo "local ip $nodeIP"

if [ ! -f $ntsCMD ]; then
    ntsCMD="$GOPATH/build/cnts"
fi

if [ ! -f $ntsCMD ]; then
    echo "not exists $ntsCMD"
    exit 1
fi

echo "stop old nts clusters"
pids=`pidof cnts`
echo $pids
kill $pids

number=0
for (( i=1; i<= $clusterNumbers; i++ ))
do
    dataDIR="$baseDIR/$i"
    echo "clear old data files"
    rm -fr $dataDIR
    mkdir -p $dataDIR
    number=$(( $sum + $i ))
    p2pPort=$[baseP2PPort+i]
    httpRPC=$[baseHTTPRPC+i]
    CMD="$ntsCMD --dev --ip $nodeIP --port $p2pPort --rpc --rpcport $httpRPC --rpcapi=admin,web3,nts,personal --bootnodes $bootNode --datadir $dataDIR --verbosity 4 console"
    echo "=====> start the node-$i, p2p:$p2pPort, httprpc:$httpRPC, execCommand:$CMD"
    dtach -n `mktemp -u $dataDIR/nerthus-socket.XXXX` $CMD 
done

dataDIR="$baseDIR/0"
rm -fr $dataDIR
mkdir -p $dataDIR
echo "start central nodes, p2p:$baseP2PPort, httprpc:$baseHTTPRPC "

CMD="$ntsCMD --dev --ip $nodeIP --port $baseP2PPort --rpc --rpcport $baseHTTPRPC --rpcapi=admin,web3,nts,personal --bootnodes $bootNode --nodekey $nodeKey --datadir $dataDIR --verbosity 4 console"
dtach -n `mktemp -u $dataDIR/nerthus-socket.XXXX` $CMD

sleep 5
echo "start witness"

accounts=(
0x485073D72Dc02C6d9F67020572B50CdBeFF40038
0x9B336cF20B2C0aC062bcbDc55844D0B8F316480e
0x54A801D279839B577e02e9CCa985119A5bD44742
0xD2E3617Ce178D87e78d7A6436d8453E86bb0dBC7
0xd29d74C4B462FAAbbE065aa69110e1cA89747824
0x7ae1ec5840d12c422bee5e89f27eadcaa8501abe
0x123dec4f462af1a65b0910438b1f7a66c1c4914c
0x8735c7e652c1d231bca00f2240f778acd539a735
0xd4aa8c8bd1e54c2e0d6ad5150e393c8a11042dbd
0x8ccaeba086507ba067a6fc353d585a16ede37e00
)

for (( i=0; i <= $clusterNumbers; i++ )) do 
    httpRPC=$[baseHTTPRPC+i]
    account=${accounts[$i]}
    if [ -z "$account" ];then 
      exit
    fi  
    body="{\"jsonrpc\":\"2.0\",\"id\":5506,\"method\":\"personal_startWitness\",\"params\":[\"$account\", \"foobar\"]}"
    echo $body
    curl "http://127.0.0.1:$httpRPC" -d "$body" 
done

