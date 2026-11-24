#!/usr/bin/env node
// Loads a running server through its ingest, the way Sentry SDKs send, until Ctrl+C, to see the
// limits and the cleanup work on a real database. Logs and events go in envelopes of their own,
// as SDKs send them, and an X-Sentry-Rate-Limits answer pauses only the categories it names, for
// that project only (an SDK drops what it would send meanwhile: "held" below).
//
//   node tools/flood/flood.mjs --project frontend=flood,backend=loop,test=events,test2=big
//   node tools/flood/flood.mjs --project frontend,backend   (scenarios given out in order)
//
// Each project runs its own scenario:
//   flood   log batches of random words (about half at error or fatal) and events of 50 issues:
//           fills the database (RATE_LIMIT_LOGS_PER_MINUTE, the cleanup by size, write speed).
//   loop    one error thrown over and over (--loop per second) and a new, different error every
//           --fresh seconds: the loop's issue counts every event but stores at most 10 a minute,
//           every new error is stored (read back through the REST API every 10 s).
//   events  different errors as fast as taken, each its own issue (RATE_LIMIT_EVENTS_PER_MINUTE).
//   big     oversized logs (a body of about 50 KB) and events (about 1.5 MB) among normal ones:
//           cut by the size limits, never refused.
// As the JS SDK does (sendClientReports), every 30 s and at Ctrl+C each project sends a client_report
// with what it held back (reason ratelimit_backoff), which the Projects page counts as dropped.
// Options: --workers 4 (envelopes in flight per flood project), --logs 200 (logs per batch),
// --words 60 (words per log), --errors 0.2 (events per flood batch), --loop 50, --fresh 10,
// --server http://localhost:8000, --data data/leveldb (its size is printed).
//
// The DSNs and the checks use the REST API with API_TOKEN (from the environment, or
// .env.local / .env in the working directory).
import { randomBytes, randomInt } from 'node:crypto';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join } from 'node:path';

const scenarios = ['flood', 'loop', 'events', 'big'];
const options = {
	project: '',
	server: 'http://localhost:8000',
	workers: 4,
	logs: 200,
	words: 60,
	errors: 0.2,
	loop: 50,
	fresh: 10,
	data: 'data/leveldb'
};

for (let index = 2; index < process.argv.length; index += 2) {
	const name = process.argv[index].replace(/^--/, '');

	if (!(name in options)) {
		console.error(`unknown option --${name}`);
		process.exit(2);
	}

	const value = process.argv[index + 1];
	options[name] = typeof options[name] === 'number' ? Number(value) : value;
}

// "frontend=flood,backend" -> [{name: 'frontend', scenario: 'flood'}, {name: 'backend', scenario: 'loop'}]
const wanted = options.project
	.split(',')
	.filter(Boolean)
	.map((entry, index) => {
		const [name, scenario = scenarios[index % scenarios.length]] = entry.split('=');

		return { name, scenario };
	});

if (!wanted.length || wanted.some(({ scenario }) => !scenarios.includes(scenario))) {
	console.error(`usage: node tools/flood/flood.mjs --project <name>[=${scenarios.join('|')}],...`);
	process.exit(2);
}

function envToken() {
	if (process.env.API_TOKEN) return process.env.API_TOKEN;

	for (const file of ['.env.local', '.env']) {
		try {
			const line = readFileSync(file, 'utf8')
				.split('\n')
				.find((entry) => entry.startsWith('API_TOKEN='));

			if (line) return line.slice('API_TOKEN='.length).trim().replace(/^["']|["']$/g, '');
		} catch {
			// no such file
		}
	}

	return '';
}

const token = envToken();

async function api(name, params = {}) {
	const query = new URLSearchParams(params).toString();
	const response = await fetch(`${options.server}/api/v1/${name}${query ? `?${query}` : ''}`, {
		headers: { Authorization: `Bearer ${token}` }
	});

	if (!response.ok) throw new Error(`${name}: ${response.status} ${await response.text()}`);

	return response.json();
}

// Letters only: numbers and ids are normalized away when messages are grouped.
const letters = 'abcdefghijklmnopqrstuvwxyz';
const word = () => Array.from(randomBytes(randomInt(3, 10)), (byte) => letters[byte % 26]).join('');

// One random buffer per sentence: a call per word made the generator, not the server, the limit.
function sentence(words) {
	const bytes = randomBytes(words * 10);
	let text = '';
	let at = 0;

	for (let index = 0; index < words; index++) {
		const length = 3 + (bytes[at++] % 7);

		if (index) text += ' ';
		for (let letter = 0; letter < length; letter++) text += letters[bytes[at++] % 26];
	}

	return text;
}
const hex = (bytes) => randomBytes(bytes).toString('hex');
const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

const logLevels = ['trace', 'debug', 'info', 'warn', 'error', 'error', 'error', 'fatal'];
const eventLevels = ['fatal', 'error', 'error', 'error', 'warning', 'info'];
const titles = Array.from({ length: 50 }, () => sentence(4));
const run = word(); // tells this run's issues apart from earlier runs'

function envelope(type, payload, header = {}) {
	return [
		JSON.stringify({ sent_at: new Date().toISOString() }),
		JSON.stringify({ type, ...header }),
		JSON.stringify(payload)
	].join('\n');
}

function logBatch(bigShare = 0) {
	const now = Date.now() / 1000;
	const traceId = hex(16);
	const items = Array.from({ length: options.logs }, () => ({
		timestamp: now,
		level: logLevels[randomInt(logLevels.length)],
		body: Math.random() < bigShare ? sentence(8000) : sentence(options.words),
		trace_id: traceId,
		attributes: { flood: { value: word(), type: 'string' } }
	}));

	return envelope('log', { items }, { item_count: items.length, content_type: 'application/vnd.sentry.items.log+json' });
}

function event(type, value, fingerprint, level, big = false) {
	return envelope('event', {
		event_id: hex(16),
		timestamp: Date.now() / 1000,
		platform: 'node',
		level,
		exception: { values: [{ type, value }] },
		fingerprint,
		contexts: { trace: { trace_id: hex(16), span_id: hex(8) } },
		// Big: many strings under 16 KB each, so the event, not a string, goes over 1 MB.
		extra: big
			? Object.fromEntries(Array.from({ length: 200 }, (_, index) => [`part${index}`, sentence(1200)]))
			: { detail: sentence(200) }
	});
}

const blank = () => ({ sent: 0, stored: 0, trimmed: 0, refused: 0, held: 0, failed: 0 });
let running = true;

// One project: its DSN, its scenario, its own counts and rate-limit pauses.
function project(name, scenario, info) {
	const dsn = new URL(info.dsn);
	const id = dsn.pathname.split('/').filter(Boolean).pop();
	const endpoint = `${dsn.protocol}//${dsn.host}/api/${id}/envelope/?sentry_key=${dsn.username}`;
	const totals = { logs: blank(), events: blank(), bytes: 0 };
	const pausedUntil = { log_item: 0, error: 0 };
	const categories = { log_item: 'logs', error: 'events' };
	const fresh = { sent: 0, accepted: 0 };
	// Held back since the last client report, per category; reported in all.
	const unreported = { log_item: 0, error: 0 };
	const reported = { log_item: 0, error: 0 };
	const loopTitle = `LoopError ${run} ${name}`;
	const freshPrefix = `FreshError ${run} ${name}`;
	const eventPrefix = `ManyError ${run} ${name}`;

	// X-Sentry-Rate-Limits: "seconds:categories:scope[:reason]", several separated by commas;
	// no categories means all of them. Retry-After counts only without it, as SDKs read them.
	function readLimits(response) {
		const header = response.headers.get('X-Sentry-Rate-Limits');
		const now = Date.now();

		if (header) {
			for (const limit of header.split(',')) {
				const [seconds, names] = limit.trim().split(':');
				const until = now + Number(seconds || 60) * 1000;

				for (const category of names ? names.split(';') : Object.keys(pausedUntil)) {
					if (category in pausedUntil) pausedUntil[category] = Math.max(pausedUntil[category], until);
				}
			}
		} else if (response.status === 429) {
			const until = now + Number(response.headers.get('Retry-After') ?? '60') * 1000;

			for (const category of Object.keys(pausedUntil)) pausedUntil[category] = until;
		}
	}

	async function send(category, body) {
		const counts = totals[categories[category]];

		if (Date.now() < pausedUntil[category]) {
			counts.held++;
			// The SDK counts what it drops, by item: a log envelope holds options.logs logs.
			unreported[category] += category === 'log_item' ? options.logs : 1;

			return false;
		}

		try {
			const response = await fetch(endpoint, {
				method: 'POST',
				body,
				headers: { 'Content-Type': 'application/x-sentry-envelope' }
			});

			await response.arrayBuffer();
			counts.sent++;
			totals.bytes += body.length;
			readLimits(response);

			if (response.ok) {
				if (response.headers.get('X-Sentry-Rate-Limits')) counts.trimmed++;
				else counts.stored++;

				return true;
			}

			if (response.status === 429) {
				counts.refused++;
			} else {
				counts.failed++;
				if (counts.failed <= 3) console.error(`${name}: answer ${response.status}: ${await response.text()}`);
			}
		} catch (error) {
			counts.failed++;
			if (counts.failed <= 3) console.error(`${name}: ${error}`);
			await sleep(1000);
		}

		return false;
	}

	// A client_report as SDKs send it: an envelope of its own, never rate limited.
	async function sendReport() {
		const discarded = Object.entries(unreported)
			.filter(([, quantity]) => quantity > 0)
			.map(([category, quantity]) => ({ reason: 'ratelimit_backoff', category, quantity }));

		if (!discarded.length) return;

		for (const { category, quantity } of discarded) unreported[category] -= quantity;

		const body = envelope('client_report', { timestamp: Date.now() / 1000, discarded_events: discarded });

		try {
			const response = await fetch(endpoint, { method: 'POST', body, headers: { 'Content-Type': 'application/x-sentry-envelope' } });

			await response.arrayBuffer();

			if (response.ok) {
				for (const { category, quantity } of discarded) reported[category] += quantity;

				return;
			}
		} catch {
			// counted again in the next report
		}

		for (const { category, quantity } of discarded) unreported[category] += quantity;
	}

	const pausedFor = (category) => Math.max(0, pausedUntil[category] - Date.now());

	async function floodWorker(bigShare) {
		while (running) {
			await send('log_item', logBatch(bigShare));

			if (Math.random() < options.errors) {
				const title = titles[randomInt(titles.length)];
				const body = event('FloodError', `${title}: ${sentence(20)}`, ['flood', title], eventLevels[randomInt(eventLevels.length)], Math.random() < bigShare);

				await send('error', body);
			}

			if (pausedFor('log_item')) await sleep(Math.min(pausedFor('log_item'), 1000));
		}
	}

	async function loopWorker() {
		let nextFresh = Date.now();

		while (running) {
			const tick = Date.now();

			for (let index = 0; index < options.loop / 10; index++) {
				void send('error', event('LoopError', `${loopTitle}: order failed`, ['flood-loop', run, name], 'error'));
			}

			if (tick >= nextFresh) {
				nextFresh = tick + options.fresh * 1000;
				fresh.sent++;

				// A different error each time: its own fingerprint and title.
				const body = event('FreshError', `${freshPrefix} ${word()} ${word()}`, ['flood-fresh', run, name, word()], 'error');

				if (await send('error', body)) fresh.accepted++;
			}

			await sleep(Math.max(0, 100 - (Date.now() - tick)));
		}
	}

	async function eventsWorker() {
		while (running) {
			const body = event('ManyError', `${eventPrefix} ${word()} ${word()}`, ['flood-many', run, name, word()], eventLevels[randomInt(eventLevels.length)]);

			await send('error', body);

			if (pausedFor('error')) await sleep(Math.min(pausedFor('error'), 1000));
		}
	}

	// What the server made of the loop: one issue counting every event, at most 10 stored a minute;
	// every new error its own issue.
	async function check() {
		if (scenario !== 'loop') return '';

		const loop = (await api('list_issues', { project: name, status: 'all', query: loopTitle, limit: '1' })).items?.[0];
		const freshIssues = (await api('list_issues', { project: name, status: 'all', query: freshPrefix, limit: '100' })).items ?? [];
		const loopText = loop
			? `loop issue ${loop.event_count} events, ${loop.stored_events} stored, ${loop.not_stored ?? 0} not stored`
			: 'loop issue not stored yet';

		return `${loopText}; new errors ${fresh.sent} sent, ${fresh.accepted} accepted, ${freshIssues.length} issues`;
	}

	function start() {
		const workers = [];

		if (scenario === 'flood' || scenario === 'big') {
			for (let index = 0; index < options.workers; index++) workers.push(floodWorker(scenario === 'big' ? 0.05 : 0));
		}
		if (scenario === 'loop') workers.push(loopWorker());
		if (scenario === 'events') for (let index = 0; index < options.workers; index++) workers.push(eventsWorker());

		return workers;
	}

	return { name, scenario, id, host: dsn.host, totals, reported, pausedFor, check, start, sendReport };
}

function directoryBytes(path) {
	let total = 0;

	try {
		for (const entry of readdirSync(path)) {
			if (entry !== 'LOG' && entry !== 'LOG.old') total += statSync(join(path, entry)).size;
		}
	} catch {
		return -1;
	}

	return total;
}

const megabytes = (bytes) => (bytes / 1024 / 1024).toFixed(1);
const counts = (name, of) =>
	`${name} ${of.sent} sent: ${of.stored} stored, ${of.trimmed} trimmed, ${of.refused} 429, ${of.held} held, ${of.failed} failed`;

function report(target) {
	const pauses = ['log_item', 'error']
		.filter((category) => target.pausedFor(category))
		.map((category) => `${category} paused ${Math.ceil(target.pausedFor(category) / 1000)}s`)
		.join(', ');

	const held = target.reported.log_item + target.reported.error;
	const reports = held ? ` | reported held ${target.reported.log_item} logs, ${target.reported.error} events` : '';

	return `  ${target.name} (${target.scenario}): ${counts('logs', target.totals.logs)} | ${counts('events', target.totals.events)} | ${megabytes(target.totals.bytes)} MB${reports}${pauses ? ` | ${pauses}` : ''}`;
}

const known = (await api('list_projects', { limit: '100' })).items;
const targets = wanted.map(({ name, scenario }) => {
	const info = known.find((item) => item.name === name || String(item.id) === name);

	if (!info) throw new Error(`no project ${name}`);

	return project(info.name, scenario, info);
});
const started = Date.now();

console.log(`run "${run}" at ${targets[0].host}; Ctrl+C stops`);
for (const target of targets) console.log(`  ${target.name} (#${target.id}): ${target.scenario}`);

const timer = setInterval(() => {
	const disk = directoryBytes(options.data);
	const sent = targets.reduce((sum, target) => sum + target.totals.bytes, 0);

	console.log(`${Math.round((Date.now() - started) / 1000)}s  ${megabytes(sent)} MB sent, disk ${disk < 0 ? '?' : `${megabytes(disk)} MB`}`);
	for (const target of targets) console.log(report(target));
}, 5000);

async function checkAll() {
	for (const target of targets) {
		const text = await target.check().catch((error) => String(error));

		if (text) console.log(`  ${target.name}: ${text}`);
	}
}

const reports = setInterval(() => targets.forEach((target) => void target.sendReport()), 30000);
const checks = targets.some((target) => target.scenario === 'loop') ? setInterval(checkAll, 10000) : undefined;

process.on('SIGINT', async () => {
	running = false;
	clearInterval(timer);
	clearInterval(checks);
	clearInterval(reports);
	await Promise.all(targets.map((target) => target.sendReport()));
	console.log(`\nstopped after ${((Date.now() - started) / 1000).toFixed(0)}s`);
	for (const target of targets) console.log(report(target));
	await checkAll();
	process.exit(0);
});

await Promise.all(targets.flatMap((target) => target.start()));
