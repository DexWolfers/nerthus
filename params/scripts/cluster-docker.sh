#/bin/bash 

localDir=$(pwd)
appPath="$GOPATH/src/gitee.com/nerthus/nerthus/cnts"
dockerAppPath="/root/cnts"
nodeKey="$localDir/nerthusnodekey"
dockerNodeKey="/root/nerthusnodekey"
nodeServers="1 2 3 4 5"
dockerImage="nerthus/nerthus-baseimage:x86_64-0.0.1"
subIPs="173.18.0.0/16"
subIP="173.18.0.10"
net="nerthus-net"
centralNode=$subIP
centralNodeContainName="central-cnts-server"
bootNodes="enode://ff85755f313aaeff5fc62ae73751224933acf6b32020b6bfb4268b411061e1e8b03c542768b77ba1b71a52958874b4994244d32c765c25872b4aee2ffd026a52@[$centralNode]:60101"
preloadJs="for (;;) { if (nts.accounts.length > 5) { personal.startWitness(nts.accounts[xxxxx], 'foobar');break;}}"
p2pPort=60101
httpPort=8655

if [ ! -f "$appPath" ]; then 
  echo "$appPath not exist"
  exit 1 
fi

if [ ! -f "$nodeKey" ]; then 
  echo "$nodeKey not exists"
  exit 1
fi

echo "central node $bootNodes"
echo "cnts path $appPath"
echo "nodeKey $nodeKey"

networkCount=`docker network  ls | grep cnts-net | wc -l`
if [ $networkCount -eq 0 ] 
then
  echo $networkCount
  echo "create cnts cluster network"
  docker network create --subnet=$subIPs $net
else 
  echo "$net already existed, ignore it"  
fi

stopContainers=""
for idx in $nodeServers
do
  containCount=`docker ps --all | grep $cnts-server$idx | wc -l`
  if [ $containCount -eq 1 ]; then
    stopContainers="$stopContainers cnts-server$idx"
  fi
done

centralCount=`docker ps --all | grep $centralNodeContainName | wc -l`
if [ $centralCount -eq 1 ]; then 
    stopContainers="$stopContainers $centralNodeContainName"
fi

if [ "$stopContainers" != "" ]; then 
  echo "====> stop $stopContainers containers"
  docker stop -t 1 $stopContainers 
  echo "====> delete $stopContainers containers"
  docker rm $stopContainers 
fi

echo "\n\n>>>>>>>>>>>>>>>>>>"
echo "start cnts cluster"
for idx in $nodeServers
do
  echo "===> start cnts-server$idx"
  nodeIP=$subIP$idx
  let witnessIdx=$idx+1
  execCommand=`echo $preloadJs | sed "s/xxxxx/$witnessIdx/g"`
  execCommand="\"$execCommand\""
  containNodeName=cnts-server$idx
  let port=$p2pPort+$idx 
  let portHttp=$httpPort+$idx
  echo "cnts preloadjs execCommand: $execCommand"
  echo "node $idx p2p port $port, http port $portHttp"
  docker run -dti --net=$net --ip=$nodeIP -w /root/ -v $appPath:$dockerAppPath \
    -p=$port:$p2pPort -p $portHttp:$httpPort \
    --name=$containNodeName $dockerImage sh -c \
    "$dockerAppPath --dev --bootnodes $bootNodes console --exec $execCommand"
done

echo "\n\n>>>>>>>>>>>>>>>>>>"
echo "start central node with console terminal for test"
execCommand=`echo $preloadJs | sed "s/xxxxx/1/g"`
execCommand="\"$execCommand\""
echo "cnts preloadjs execCommand: $execCommand"
echo "central node p2p port $p2pPort, http port $httpPort"

docker run -dti --net=$net --ip=$centralNode -w /root/ \
  -v $nodeKey:$dockerNodeKey -v $appPath:$dockerAppPath \
  -p=$p2pPort:$p2pPort -p $httpPort:$httpPort \
  --name=$centralNodeContainName $dockerImage \
  sh -c "$dockerAppPath --dev --bootnodes $bootNodes --nodekey $dockerNodeKey console --exec $execCommand"

echo "-----------------------------"
echo "central node start info just do it"
echo "docker logs $centralNodeContainName" 
