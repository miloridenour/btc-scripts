import { promises as fs } from 'fs';

class ConfigNotFoundError extends Error {
	constructor(message = 'config file not found') {
		super(message);
		this.name = 'ConfigNotFoundError';
	}
}

const CONFIG_FILE = 'data/config/identityConfig.json';

/**
 * Serializes the config object and writes it to data/config/identityConfig.json
 * Creates the directory structure if it doesn't exist
 * @param {Object} config - The configuration object to save
 * @returns {Promise<void>}
 * @throws {Error} If marshaling or writing fails
 */
async function saveConfig(config) {
	try {
		const data = JSON.stringify(config, null, 2);

		// Create directory structure if it doesn't exist
		const dir = 'data/config';
		await fs.mkdir(dir, { recursive: true });

		await fs.writeFile(CONFIG_FILE, data, { mode: 0o644 });
	} catch (err) {
		if (err.message.includes('JSON')) {
			throw new Error(`failed to marshal config: ${err.message}`);
		}
		throw new Error(`failed to write config file: ${err.message}`);
	}
}

/**
 * Reads and deserializes data/config/identityConfig.json into an object
 * If the file doesn't exist, it creates one with default values
 * @param {Object} defaultConfig - The default configuration object
 * @returns {Promise<Object>} The loaded configuration
 * @throws {ConfigNotFoundError} If the file didn't exist and was created
 * @throws {Error} If reading or unmarshaling fails
 */
async function loadConfig(defaultConfig) {
	try {
		// Check if file exists
		await fs.access(CONFIG_FILE);

		// File exists, read it
		const data = await fs.readFile(CONFIG_FILE, 'utf8');

		try {
			return JSON.parse(data);
		} catch (err) {
			throw new Error(`failed to unmarshal config: ${err.message}`);
		}
	} catch (err) {
		if (err.code === 'ENOENT') {
			// File doesn't exist, create it with default values
			try {
				await saveConfig(defaultConfig);
			} catch (saveErr) {
				throw new Error(`failed to create default config: ${saveErr.message}`);
			}
			throw new ConfigNotFoundError();
		}

		// Re-throw if it's not a "file not found" error
		if (err instanceof ConfigNotFoundError) {
			throw err;
		}
		throw new Error(`failed to read config file: ${err.message}`);
	}
}

export { ConfigNotFoundError, saveConfig, loadConfig };
