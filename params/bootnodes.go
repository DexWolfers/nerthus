// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package params

// MainnetBootnodes are the enode URLs of the P2P bootstrap nodes running on
// the main Ethereum network.
var MainnetBootnodes = []string{
	"enode://e7da96d0bc46057a3c63f5b6ae4414998a5379d1c944cc63d764def9580f9202e458992090dfbe4a61198dbe68667a414f1e4e8722b1ab8143d681809a379fd6@159.138.91.167:60102",
	"enode://c717ce653dcc20a7438634447f8b0140ab26be912b5d022009bf11db9ee6231782963ec0bb0131ebb33cd9efa84c42b6f0cad7fd58e08d28ce61321b404c4870@159.138.97.218:60101",
	"enode://4a9f1a1275db83f4ff04fb72302b27a156da114e6a362e693ebef35a3a3f96f5972daa363c6d308f13cab8d0713c775e01a94912638c1bc1eab01564161a40f9@159.138.110.70:60101",
}

// TestnetBootnodes are the enode URLs of the P2P bootstrap nodes running on the  testnet
var TestnetBootnodes = []string{}

var DevBootnodes = []string{}

// RinkebyV5Bootnodes are the enode URLs of the P2P bootstrap nodes running on the
// Rinkeby test network for the experimental RLPx v5 topic-discovery network.
var RinkebyV5Bootnodes = []string{}

// DiscoveryV5Bootnodes are the enode URLs of the P2P bootstrap nodes for the
// experimental RLPx v5 topic-discovery network.
var DiscoveryV5Bootnodes = []string{}
