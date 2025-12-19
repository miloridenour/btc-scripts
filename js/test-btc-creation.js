'use strict';
Object.defineProperty(exports, '__esModule', { value: true });
var bitcoin = require('bitcoinjs-lib');
var crypto = require('crypto');
function createP2WSHAddress(publicKey, tag, network) {
	if (network === void 0) {
		network = bitcoin.networks.bitcoin;
	}
	// Convert public key to Buffer if it's a hex string
	var pubKeyBuffer = typeof publicKey === 'string' ? Buffer.from(publicKey, 'hex') : publicKey;
	// Create a witness script that includes the tag and public key
	// This is a simple example: OP_PUSHDATA(tag) OP_DROP <pubkey> OP_CHECKSIG
	var tagBuffer = Buffer.from(tag, 'hex');
	var witnessScript = bitcoin.script.compile([
		pubKeyBuffer,
		bitcoin.opcodes.OP_CHECKSIGVERIFY,
		tagBuffer
	]);
	// Create P2WSH payment object
	var p2wsh = bitcoin.payments.p2wsh({
		redeem: {
			output: witnessScript,
			network: network
		},
		network: network
	});
	if (!p2wsh.address) {
		throw new Error('Failed to generate P2WSH address');
	}
	return {
		address: p2wsh.address,
		witnessScript: witnessScript,
		scriptHash: p2wsh.hash
	};
}
var depositInstruction = 'deposit_to=hive:milo-hpr';
var tag = crypto.hash('sha256', depositInstruction);
var address = createP2WSHAddress(
	'0242f9da15eae56fe6aca65136738905c0afdb2c4edf379e107b3b00b98c7fc9f0',
	tag,
	bitcoin.networks.testnet
).address;
console.log(address);
