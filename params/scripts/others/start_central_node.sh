#/bin/bash

ifconfig

echo $NTS_BOOT_NODES
echo $NTS_NODEKEY
./root/cnts --dev --bootnodes $NTS_BOOT_NODES --nodekey $NTS_NODEKEY console --exec "personal.startWitness(nts.accounts[$NTS_PREJS_IDX], 'foobar')" 
