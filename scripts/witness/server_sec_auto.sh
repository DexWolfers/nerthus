#!/bin/bash  
name="nerthus"
pw="input your pw"
adduser $name
(
    echo "$pw"
    echo "$pw"
)| passwd $name --stdin

envpath="/home/$name/.nerthus"
workspace="/datadisk/$name"
mkdir -p $workspace
mkdir -p $envpath
echo "workspace=$workspace" >> "/home/$name/.nerthus/env.sh"

chown  $name:$name $workspace 
chown  $name:$name $envpath
chmod  700 $envpath

echo "successful!"
