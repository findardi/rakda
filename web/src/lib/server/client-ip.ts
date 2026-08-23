import { AsyncLocalStorage } from 'node:async_hooks';

const storage = new AsyncLocalStorage<string>();

export function runWithClientIP<T>(ip: string | undefined, fn: () => T): T {
	if (ip === undefined) return fn();
	return storage.run(ip, fn);
}

export function getClientIP(): string | undefined {
	return storage.getStore();
}
