#!/bin/bash

print_help(){
    echo "manager nerthus testnet env"
    echo "\`$0 --init\` init testnet env config"
    echo "\`$0 --install\` install nerthus soft and install nerthus service"
    echo "\`$0 --start\` run node and connect to testnet"
    echo "\`$0 --start_witness\` start wintess server on local node"
    echo "\`$0 --attach\` connect to local node console"
}

envpath="$HOME/.nerthus"
if [ ! -d "$envpath" ];then
   mkdir -p "$envpath" 
fi;
envfile="$envpath/env.sh"
loadenv(){
    if [ -f "$envfile" ];then
        source "$envfile" 
    fi;
}
loadenv

# cnts use
export NERTHUS_WS_PATH=$workspace

# 初始化环境信息
init(){
    echo "init nerthus env"
    echo "will save config info to $envfile file"
    echo "# nerthus node running info" > $envfile
    read -p "Enter witness address:" witness
    echo "witness=$witness" >> $envfile
    read -p "Enter witness keystore password:" password
    echo "password=$password # witness keystore password" >> $envfile

    read -p "Enter nerthus workspace dir:" workspace
    echo "workspace=$workspace" >> $envfile
     
    # mkdir -p "$ws" #设置目录
    # echo "export NERTHUS_WS_PATH=$ws" >> $HOME/.bash_profile
    # source $HOME/.bash_profile 
}  

# 检查命令返还的 code ,如果不是 0，则认为执行失败而退出
ckrcode(){
    c=$?; if [[ $c != 0 ]]; then exit $c; fi
}

# 安装 nerthus
install(){
    # release="https://github.com/nerthus-foundation-ltd/nerthus/releases/download/untagged-aac99c1bac2790287e78/cnts-linux.zip"
    # wget -O nerthusbin.gz "$release"
    # ckrcode 
    # # 解压到 $HOME/.nerthus/env
    # tar -zxvf nerthusbin.gz -C $envpath
    # ckrcode

    $envpath/cnts version
    ckrcode 
    echo "install done" 
}


# 运行 cnts 
start(){
    maxpeers=25
    if [[ ! -z "$witness" ]]; then
       maxpeers=30
    elif [[ ! -z "$council" ]]; then
       maxpeers=60
    fi

    declare -a run_params=(
        "--testnet"
        "--datadir $workspace"
        "--config $envpath/cnts.testnet.yaml"
        "--nat extip:$ip"
        "--maxpeers $maxpeers"
        "--rpc --rpcapi * --rpcaddr 127.0.0.1" #for debug
    )
   echo "run cmd: cnts ${run_params[*]}"
   $envpath/cnts ${run_params[*]}
   ckrcode
}

monitor_work_status(){
    while true;do
        status=`systemctl is-active cnts`
        if [ $status == "active" ]; then
            start_witness
            sleep 10s
        else
            sleep 1s
        fi
        loadenv
    done    
}

start_witness(){
    js=""
    if [[ ! -z "$witness" ]]; then
        js="witness.startWitness('$witness','$password')"
    elif [[ ! -z "$council" ]]; then
       js="witness.startMember('$council','$password')"
    else
        echo "config(witness|council) is empty"
        return   
    fi

    ipc="$workspace/testnet/cnts.ipc"

    declare -a runattach_params=(
            "--testnet"
            "--datadir $workspace"
            "--endpoint $ipc"
            "--exec $js"
    )
    $envpath/cnts attach ${runattach_params[*]} 
}

attach(){
   $envpath/cnts attach --testnet 
}
case $1 in
  --install) install;;
  --init ) init;;
  --start ) start;;
  --start_witness) start_witness;;
  --attach ) attach;;
  --monitor ) monitor_work_status;;
  *) echo "Invalid option"; print_help; exit 0;
esac

