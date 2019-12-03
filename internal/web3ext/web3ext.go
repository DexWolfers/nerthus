// Author: @ysqi

// package web3ext contains cnts specific web3.js extensions.
package web3ext

var Modules = map[string]string{
	"admin":    Admin_JS,
	"nts":      NTS_JS,
	"personal": Personal_JS,
	"witness":  Witness_JS,
}

type PropertyInfo struct {
	Name      string
	Call      string
	Formatter string
}

var Propertys = map[string][]PropertyInfo{
	"nts": {
		{
			Name: "version.node",
			Call: "web3_clientVersion",
		},
		{
			Name:      "version.network",
			Call:      "net_version",
			Formatter: "web3.utils.toDecimal",
		},
		{
			Name:      "version.nerthus",
			Call:      "nts_protocolVersion",
			Formatter: "web3.utils.toDecimal",
		},
		{
			Name: "systemChain",
			Call: "nts_systemChain",
		},
		{
			Name: "genesisHash",
			Call: "nts_genesisHash",
		},
	},
	"personal": {},
	"admin": {
		{
			Name: "nodeInfo",
			Call: "admin_nodeInfo",
		},
		{
			Name: "datadir",
			Call: "admin_datadir",
		},
		{
			Name: "version",
			Call: "admin_version",
		},
	},
}

const NTS_JS = `
web3.extend({
	property: 'nts',
	methods:
	[ 
		{
			name: 'accounts',
			call: 'personal_listAccounts',
		}, 
		{
			name: 'getWitnessGroups',
			call: "nts_getWitnessGroups",
			params: 1,
            inputFormatter: [web3.extend.formatters.inputDefaultBlockNumberFormatter],
		},
		{
			name: 'getWitnessGroupInfo',
			call: "nts_getWitnessGroupInfo",
			params: 2,
			inputFormatter: [null, web3.extend.formatters.inputDefaultBlockNumberFormatter]
		}
	]
});
`

const Personal_JS = `
web3.extend({
	property: 'nts.personal',
	methods:
	[
		{
			name: 'wallets',
			call: 'personal_listWallets',
		},
		{
			name: 'openWallet',
			call: 'personal_openWallet',
			params: 2
		},
		{
			name: 'deriveAccount',
			call: 'personal_deriveAccount',
			params: 3,
			
		}
	]
})
`

const Witness_JS = `
web3.extend({
	property: 'witness',
	methods:
	[
		{
			name: 'startWitness',
			call: 'witness_startWitness',
			params: 2,
			inputFormatter: [web3.extend.formatters.inputAddressFormatter, null],
		}, 
		{
			name: 'stopWitness',
			call: 'witness_stopWitness',
			params: 0,
		},
		{
			name: 'startMember',
			call: 'witness_startMember',
			params: 2,
			inputFormatter: [web3.extend.formatters.inputAddressFormatter, null],
		},
		{
			name: 'stopMember',
			call: 'witness_stopMember',
			params: 0,
		},
		{
			name: 'getWitnessFee',
			call: 'witness_getWitnessFee',
			params: 1,
			inputFormatter: [web3.extend.formatters.inputAddressFormatter],
		},
		{
			name: 'getWitnessFee',
			call: 'witness_getWitnessFee',
			params: 1,
			inputFormatter: [web3.extend.formatters.inputAddressFormatter],
		},
		{
			name:'status',
			call:"witness_status",
 			params: 0,
		}
	]
})
`

const Admin_JS = `
web3.extend({
	property: 'admin',
	methods:
	[
		{
			name:"peers",
			call:"admin_peers",
		},
		{
			name: 'addPeer',
			call: 'admin_addPeer',
			params: 1
		},
		{
			name: 'removePeer',
			call: 'admin_removePeer',
			params: 1
		}, 
		{
			name: 'startRPC',
			call: 'admin_startRPC',
			params: 4,
			inputFormatter: [null, null, null, null]
		},
		{
			name: 'stopRPC',
			call: 'admin_stopRPC'
		},
		{
			name: 'startWS',
			call: 'admin_startWS',
			params: 4,
			inputFormatter: [null, null, null, null]
		},
		{
			name: 'stopWS',
			call: 'admin_stopWS'
		}
	]
});
`
