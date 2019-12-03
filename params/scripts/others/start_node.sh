#/bin/bash
#./root/cnts --dev --bootnodes $NTS_BOOT_NODES console --exec "personal.startWitness(nts.accounts[$NTS_PREJS_IDX], 'foobar')" 
ifconfig
echo $NTS_BOOT_NODES
#ping 172.16.238.10
startWitnessPrefix='for (;;) { if (nts.accounts.length > 0) { personal.startWitness(nts.accounts[' 
startWitnessSubfix='], "foobar");break;}}'
startWitness="{$startWitnessPrefix}{$NTS_PREJS_IDX}{$startWitnessSubfix}"
./root/cnts --dev --bootnodes $NTS_BOOT_NODES console --exec $startWitness  
