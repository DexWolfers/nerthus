#!/bin/bash
ntsCMD="$GOPATH/src/gitee.com/nerthus/nerthus/build/bin/cnts"

baseP2PPort="61101"
baseHTTPRPC="8755"
baseDIR="/tmp/nerthus_cluster"
clusterNumbers=3
nodeKey="nerthusnodekey"
yaml="cnts.dev.yaml"
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

# dataDIR="$baseDIR/0"
# rm -fr $dataDIR
# mkdir -p $dataDIR
# echo "start central nodes, p2p:$baseP2PPort, httprpc:$baseHTTPRPC "
# #CMD="$ntsCMD --dev --ip $nodeIP --port $baseP2PPort --rpc --rpcport $baseHTTPRPC --rpcapi=admin,web3,nts,personal --config $yaml --bootnodes $bootNode --nodekey $nodeKey --datadir $dataDIR --verbosity 4 console"
# CMD="$ntsCMD --dev --ip $nodeIP --port $baseP2PPort --rpc --rpcport $baseHTTPRPC --rpcapi=admin,web3,nts,personal --config $yaml --nodekey $nodeKey --datadir $dataDIR --verbosity 5 console"
# echo "=====> start the node, $CMD"
# nohup $CMD > "$dataDIR/console.log" &
# /Users/rg/go/src/gitee.com/nerthus/nerthus/build/bin/cnts --dev --ip 192.168.3.22 --port 61103 --rpc --rpcport 8757 --rpcapi=admin,web3,nts,personal --config cnts.dev.yaml --bootnodes 'enode://ff85755f313aaeff5fc62ae73751224933acf6b32020b6bfb4268b411061e1e8b03c542768b77ba1b71a52958874b4994244d32c765c25872b4aee2ffd026a52@[192.168.3.22]:61101' --datadir /tmp/nerthus_cluster/2 verbosity 5

for (( i=1; i<= $clusterNumbers; i++ ))
do
    dataDIR="$baseDIR/$i"
    echo "clear old data files: $dataDIR"
    rm -fr $dataDIR
    mkdir -p $dataDIR
    p2pPort=$[baseP2PPort+i]
    httpRPC=$[baseHTTPRPC+i]
    CMD="$ntsCMD --dev --ip $nodeIP --port $p2pPort --rpc --rpcport $httpRPC --rpcapi=admin,web3,nts,personal --config $yaml --bootnodes $bootNode --datadir $dataDIR --verbosity 4"
    echo "=====> start the node-$i, p2p:$p2pPort, httprpc:$httpRPC, execCommand:$CMD"
    nohup $CMD > "$dataDIR/console.log" &
done

sleep 10
echo "start witness"

accounts=(
0x485072D72Dc02C6d9F67020572B50CdBeFF40038
0x9B336cF20B2C0aC062bcbDc55844D0B8F316480e
0x54A801D279839B577e02e9CCa985119A5bD44742
0xD2E3617Ce178D87e78d7A6436d8453E86bb0dBC7
0xd29d74C4B462FAAbbE065aa69110e1cA89747824
0xc9882941bB343c833e182Dd20d3fbF81DEf096C2
0x06f6977f1662552E54A55BE27e59210B92B42251
0x54F9de9E878866cAd0792cc726c93391e622187C
0xa0CC4BCa43D62A7f1249BF04B1209eA90E2A0479
0xBc9aa06ca464032bDb7DAe241A40c10E96ce6F87
0xAb362237835871EddE620a8F58eB1F8D1D01d322
)

for (( i=1; i <= $clusterNumbers; i++ )) do
    httpRPC=$[baseHTTPRPC+i]
    account=${accounts[$i]}
    if [ -z "$account" ];then 
      exit
    fi  
    body="{\"jsonrpc\":\"2.0\",\"id\":5506,\"method\":\"personal_startWitness\",\"params\":[\"$account\", \"foobar\"]}"
    echo $body
    curl "http://127.0.0.1:$httpRPC" -d "$body"
    #echo "http://127.0.0.1:$httpRPC -d $body"
done

