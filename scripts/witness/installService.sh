#!/bin/bash

# 安装为服务
user="nerthus"
currfile="/home/nerthus/.nerthus/witness_cli.sh"

chmod 700 $currfile

echo "
[Unit]
Description=Nerthus Node
[Service]
User=$user
Group=nerthus
Type=simple
ExecStart=$currfile --start
ExecStartPost=systemctl restart cntswork
Restart=on-failure
[Install]
WantedBy=multi-user.target
Alias=cnts
" > /etc/systemd/system/cnts.service

echo "
[Unit]
Description=Nerthus Witnesss Service
Requires=cnts.service
[Service]
User=$user
Group=nerthus
Type=simple
ExecStart=$currfile --monitor
Restart=on-failure
[Install]
WantedBy=multi-user.target
Alias=cntswork
" > /etc/systemd/system/cntswork.service


echo "instlled cnts service done. you can use systemctl commond control."
systemctl enable cnts
systemctl enable cntswork 