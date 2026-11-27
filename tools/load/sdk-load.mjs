// Loads a server through the real @sentry/node SDK: node tools/load/sdk-load.mjs <dsn> <seconds> <worker index>.
// Needs @sentry/node: SENTRY_NODE_DIR is a directory whose node_modules has it (default ../sentry-playground).
// Each event is an error thrown through a few frames, with breadcrumbs, a user, tags and
// contexts as the SDK adds them; 1 in 4 repeats one of 50 issues, the rest are new issues
// (a new error class per event). Every error also writes LOGS logs (5 by default) through Sentry.logger.
import { createRequire } from 'node:module';

const sdkDir = process.env.SENTRY_NODE_DIR ?? new URL('../../../sentry-playground', import.meta.url).pathname;
const require = createRequire(`${sdkDir}/package.json`);
const Sentry = require('@sentry/node');

const [dsn, seconds, worker] = process.argv.slice(2);
const logsPerError = Number(process.env.LOGS ?? 5);
const stats = { captured: 0, logs: 0, envelopes: 0, status: {}, refused: {} };

// The SDK's own transport, counting the answers.
function countingTransport(options) {
	const transport = Sentry.makeNodeTransport(options);

	return {
		...transport,
		send: async (envelope) => {
			const response = await transport.send(envelope);
			const status = response?.statusCode ?? 'none';
			stats.envelopes++;
			stats.status[status] = (stats.status[status] ?? 0) + 1;

			if (status === 429) {
				const header = response.headers?.['x-sentry-rate-limits'] ?? 'none';
				const kind = envelope[1].map((item) => item[0].type).join('+');
				const key = `${kind} -> ${header}`;
				stats.refused[key] = (stats.refused[key] ?? 0) + 1;
			}

			return response;
		}
	};
}

Sentry.init({
	dsn,
	enableLogs: logsPerError > 0,
	environment: 'load',
	release: 'load@1.0.0',
	sendClientReports: true,
	transport: countingTransport,
	tracesSampleRate: 0,
	integrations: (defaults) => defaults.filter((integration) => integration.name !== 'Http')
});

const words = 'order payment invoice user session cart product shipment refund coupon account report'.split(' ');
const pick = (list) => list[Math.floor(Math.random() * list.length)];

function classFor(index) {
	const name = `LoadError${index}`;

	// The SDK reports error.name as the exception type, which grouping reads.
	return class extends Error {
		constructor(message) {
			super(message);
			this.name = name;
		}
	};
}

const repeated = Array.from({ length: 50 }, (_, index) => classFor(`Repeated${worker}x${index}`));
let fresh = 0;

function validate(record, ErrorClass) {
	if (record.total > 0) throw new ErrorClass(`Cannot process ${pick(words)} ${record.id}: total ${record.total} is over the limit`);
}

function handle(record, ErrorClass) {
	return validate(record, ErrorClass);
}

function processRequest(ErrorClass) {
	const record = { id: Math.floor(Math.random() * 1e6), total: 1 + Math.random() * 100 };

	return handle(record, ErrorClass);
}

function once() {
	const ErrorClass = Math.random() < 0.25 ? pick(repeated) : classFor(`W${worker}N${fresh++}`);

	for (let index = 0; index < 10; index++) {
		Sentry.addBreadcrumb({ category: 'http', type: 'http', data: { method: 'GET', url: `/api/${pick(words)}/${index}`, status_code: 200 } });
	}

	Sentry.setUser({ id: String(Math.floor(Math.random() * 1000)), email: 'user@example.com' });
	Sentry.setTag('route', `/api/${pick(words)}`);

	for (let index = 0; index < logsPerError; index++) {
		Sentry.logger.info(`Handled ${pick(words)} ${index} for request ${Math.floor(Math.random() * 1e6)}`, { route: pick(words) });
		stats.logs++;
	}

	try {
		processRequest(ErrorClass);
	} catch (error) {
		Sentry.captureException(error);
		stats.captured++;
	}
}

const end = Date.now() + Number(seconds) * 1000;

// A batch, then a turn of the event loop so the SDK's transport sends (its buffer drops the rest).
while (Date.now() < end) {
	for (let index = 0; index < 20; index++) once();

	await new Promise((resolve) => setTimeout(resolve, 2));
}

await Sentry.flush(10000);
console.log(JSON.stringify({ worker, ...stats }));
process.exit(0);
