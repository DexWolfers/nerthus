#!/bin/bash 
ckrcode(){
    c=$?; if [[ $c != 0 ]]; then exit $c; fi
}

setFirewall(){
    firewall-cmd --zone=public --add-port=22/tcp --permanent
    firewall-cmd --zone=public --add-port=60101-60102/tcp --permanent
    firewall-cmd --zone=public --add-port=60101-60102/udp --permanent
    systemctl enable firewalld
    systemctl start firewalld
    firewall-cmd --reload
}

newUser(){
    read -p "Enter new user name(skip when empty):" name
    if [[ ! -z "$name" ]];then
        adduser $name
        ckrcode
        passwd  $name 
        ckrcode
    else
        read -p "Enter user name for set:" name 
    fi
}

newUser

read -p "Enter nerthus workspace dir:" workspace
mkdir -p $workspace
ckrcode
chown  $name:$name $workspace 
ckrcode

echo "Switching user to $name"
sudo -u "$name" -i /bin/bash - <<-'EOF'
    envpath="$HOME/.nerthus" 
    mkdir -p "$envpath" 
    echo "workspace=$workspace" >> "$envpath/env.sh"
EOF
echo
ckrcode

read -p "Enable firewall?" -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]
then
  setFirewall
  ckrcode
fi

echo "successful!"
