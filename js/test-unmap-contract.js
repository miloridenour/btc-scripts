import { Aioha, KeyTypes, Providers } from '@aioha/aioha';
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

		console.log(
			await aioha.vscCallContract(
				'vsc1BTpUPXMyvc6LNe38w5UNCNAURZHH6esBic',
				'unmap',
				``,
				10000,
				[],
				KeyTypes.Active
			)
		);
	} catch (err) {
		if (err instanceof ConfigNotFoundError) {
			console.log('Created new config file.');
		} else {
			console.error('Error:', err.message);
		}
	}
}

main();
