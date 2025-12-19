import * as bitcoin from 'bitcoinjs-lib';
import * as crypto from 'crypto';

function createP2WSHAddress(
	publicKey: string,
	tag: string,
	network: bitcoin.Network = bitcoin.networks.bitcoin
): {
	address: string;
	witnessScript: Uint8Array<ArrayBufferLike>;
	scriptHash: Uint8Array<ArrayBufferLike>;
} {
	// Convert public key to Buffer if it's a hex string
	const pubKeyBuffer = typeof publicKey === 'string' ? Buffer.from(publicKey, 'hex') : publicKey;

	// Create a witness script that includes the tag and public key
	// This is a simple example: OP_PUSHDATA(tag) OP_DROP <pubkey> OP_CHECKSIG
	const tagBuffer = Buffer.from(tag, 'hex');

	const witnessScript = bitcoin.script.compile([
		pubKeyBuffer,
		bitcoin.opcodes.OP_CHECKSIGVERIFY,
		tagBuffer
	]);

	// Create P2WSH payment object
	const p2wsh = bitcoin.payments.p2wsh({
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
		scriptHash: p2wsh.hash!
	};
}

const depositInstruction = 'deposit_to=hive:milo-hpr';
const tag = crypto.hash('sha256', depositInstruction);

let { address } = createP2WSHAddress(
	'0242f9da15eae56fe6aca65136738905c0afdb2c4edf379e107b3b00b98c7fc9f0',
	tag,
	bitcoin.networks.testnet
);

console.log(address);
