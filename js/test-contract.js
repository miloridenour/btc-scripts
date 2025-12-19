import { Aioha, Providers } from '@aioha/aioha';
import { PlaintextKeyProvider } from '@aioha/aioha/build/providers/custom/plaintext.js';
import { ConfigNotFoundError, loadConfig } from './config.js';

async function main() {
	const aioha = new Aioha(); // or initAioha()

	const defaultConfig = {
		hiveUsername: 'INSERT_USERNAME',
		activeKey: 'INSERT_ACTIVE_KEY'
	};

	try {
		const config = await loadConfig(defaultConfig);

		aioha.registerCustomProvider(new PlaintextKeyProvider(config.activeKey));
		const login = await aioha.login(Providers.Custom, config.hiveUsername, {});

		// console.log(
		//   await aioha.vscCallContract(
		//     "vsc1BcS12fD42kKqL2SMLeBzaEKtd9QbBWC1dt",
		//     "seed_blocks",
		//     `{"block_header":"00000020e5bf9b840af7aeef235e751474b7aaf603b7a5ac42e7c8dc1c00d90d000000005b5742c272944eec28c71f241855c500b3f921bc496da5fa98fbda42bb768778e123ef68ffff001d865c9567","block_height":4737010}`,
		//     10000,
		//     [],
		//     KeyTypes.Active
		//   )
		// );

		// console.log(
		//   await aioha.vscCallContract(
		//     "vsc1BcS12fD42kKqL2SMLeBzaEKtd9QbBWC1dt",
		//     "add_blocks",
		//     `{"blocks":"","latest_fee":1}`,
		//     10000,
		//     [],
		//     KeyTypes.Active
		//   )
		// );

		// console.log(
		// 	await aioha.vscCallContract(
		// 		'vsc1BVgE4NL3nZwtoDn82XMymNPriRUp9UVAGU',
		// 		'create_key_pair',
		// 		``,
		// 		10000,
		// 		[],
		// 		KeyTypes.Active
		// 	)
		// );

		console.log('config loaded', config);
	} catch (err) {
		if (err instanceof ConfigNotFoundError) {
			console.log('Created new config file.');
		} else {
			console.error('Error:', err.message);
		}
	}
}

main();
